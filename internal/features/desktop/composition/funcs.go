// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"

	companionwails "raven/internal/features/companion/adapters/wails"
	configuration "raven/internal/features/configuration/domain"
	desktopwails "raven/internal/features/desktop/adapters/wails"
	"raven/internal/features/desktop/ports"
	"raven/internal/shared/lifetime"
)

func newApplication(params appParams, settings configuration.Settings) *application.App {
	return desktopwails.NewApplication(&desktopwails.ApplicationDependencies{
		ApplicationID: settings.ApplicationID(), Assets: params.Assets.Files,
		Icon: params.Assets.Icon, Middleware: params.Media.Middleware,
		SecondInstance: params.Visibility.SecondInstance, Logger: params.Logger,
	})
}

func newDesktop(params desktopParams, settings configuration.Settings) *desktopwails.Desktop {
	deps := desktopDependencies(params)

	deps.ShowReviews, deps.LogErrorKey = settings.ShowReviews(), settings.LogErrorKey()

	desktop := desktopwails.New(deps)
	params.Lifecycle.Append(desktopLifecycle(desktop, params.Runtime, params.Requests))

	return desktop
}

func desktopDependencies(params desktopParams) *desktopwails.Dependencies {
	var deps desktopwails.Dependencies

	deps.App, deps.Main = params.App, params.Handles.Main
	deps.Widget, deps.Review = params.Handles.Widget, params.Handles.Review
	deps.OnReview, deps.Icon = params.Notifications.OnReview, params.Assets.Icon
	bindDesktopStartup(&deps, params)
	bindDesktopWindows(&deps, params)
	bindDesktopReminders(&deps, params)

	return &deps
}

func bindDesktopStartup(deps *desktopwails.Dependencies, params desktopParams) {
	deps.ApplicationStarted, deps.RequestOpen = params.Visibility.ApplicationStarted, params.Visibility.RequestOpen
	deps.InitializeStartup, deps.RefreshStartup = params.Startup.Initialize, params.Startup.Refresh
	deps.ToggleStartup, deps.SetStartupPublisher = params.Startup.Toggle, params.Startup.SetPublisher
}

func bindDesktopWindows(deps *desktopwails.Dependencies, params desktopParams) {
	if params.Windows == nil {
		return
	}

	deps.ShowMain, deps.HideMain = params.Windows.ShowMain, params.Windows.HideMain
	deps.ConfigureWidget, deps.ResetWidget = params.Windows.ConfigureWidget, params.Windows.ResetWidget
	deps.PositionFloatingReview = params.Windows.PositionFloatingReview
	deps.RequestFloatingReviewClose, deps.Quit = params.Windows.RequestFloatingReviewClose, params.Windows.Quit
}

func bindDesktopReminders(deps *desktopwails.Dependencies, params desktopParams) {
	if params.Reminders == nil {
		return
	}

	deps.StartReminders, deps.WakeReminders = params.Reminders.Start, params.Reminders.Wake
}

//nolint:contextcheck // The request loop must outlive the bounded startup context.
func desktopLifecycle(
	desktop *desktopwails.Desktop,
	runtime *lifetime.Runtime,
	requests ports.OpenRequests,
) fx.Hook {
	var done <-chan struct{}

	return fx.Hook{
		OnStart: func(context.Context) error {
			done = openDesktopRequests(desktop, runtime, requests)

			return nil
		},

		OnStop: func(ctx context.Context) error {
			desktop.Dispose()
			runtime.Cancel()

			return waitForDesktop(ctx, done)
		},
	}
}

func openDesktopRequests(
	desktop *desktopwails.Desktop,
	runtime *lifetime.Runtime,
	requests ports.OpenRequests,
) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)

		desktop.Open(runtime.Context(), requests.Pending())
	}()

	return done
}

func waitForDesktop(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop desktop requests: %w", ctx.Err())
	}
}

func configureDesktop(desktop *desktopwails.Desktop) { desktop.Configure() }

// NewBridge binds graph control before the native event loop starts.
func NewBridge(graph Lifecycle, runtime *lifetime.Runtime) *Bridge {
	var (
		startOnce, stopOnce sync.Once
		startErr, stopErr   error
	)

	return &Bridge{lifecycle: func(ctx context.Context, starting bool) error {
		if starting {
			startOnce.Do(func() { startErr = startBridge(ctx, runtime, graph.Start) })

			return errors.Join(startErr)
		}

		stopOnce.Do(func() { stopErr = stopBridge(ctx, runtime.Cancel, graph.Stop) })

		return errors.Join(stopErr)
	}}
}

func startBridge(
	ctx context.Context,
	runtime *lifetime.Runtime,
	start func(context.Context) error,
) error {
	runtime.Attach(ctx)

	startup, cancel := context.WithTimeout(ctx, lifecycleTimeout)

	defer cancel()

	err := isolate(start(startup))
	if err != nil {
		runtime.Cancel()
	}

	return errors.Join(err)
}

func stopBridge(ctx context.Context, cancelRuntime func(), stop func(context.Context) error) error {
	cancelRuntime()

	shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), lifecycleTimeout)

	defer cancel()

	return errors.Join(isolate(stop(shutdown)))
}

// ServiceStartup starts the graph with an independent worker lifetime.
func (bridge *Bridge) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	return errors.Join(bridge.lifecycle(ctx, true))
}

// ServiceShutdown releases the graph once, including after an unsuccessful Run.
func (bridge *Bridge) ServiceShutdown() error {
	return errors.Join(bridge.lifecycle(context.Background(), false))
}

func isolate(err error) error {
	if err != nil {
		return &lifecycleError{message: err.Error()}
	}

	return nil
}

func (failure *lifecycleError) Error() string { return failure.message }

func desktopConfiguration(settings configuration.Settings) *desktopwails.Configuration {
	layout := configuration.Layout(settings)

	return &desktopwails.Configuration{
		ApplicationID: settings.ApplicationID(), WidgetSize: layout.WidgetSize,
		ReviewWidth: layout.ReviewWidth, ReviewHeight: layout.ReviewHeight,
	}
}

func newWindows(app *application.App, settings configuration.Settings) companionwails.WindowSet {
	return desktopwails.NewWindows(app, desktopConfiguration(settings))
}
