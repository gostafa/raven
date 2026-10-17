// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"

	companionwails "raven/internal/features/companion/adapters/wails"
	configuration "raven/internal/features/configuration/domain"
	desktopwails "raven/internal/features/desktop/adapters/wails"
	"raven/internal/features/desktop/ports"
	"raven/internal/shared/lifetime"
)

func newApplication(params appParams, settings *configuration.Settings) *application.App {
	return desktopwails.NewApplication(&desktopwails.ApplicationDependencies{
		Configuration: desktopConfiguration(settings),
		Assets:        params.Assets,
		Media:         params.Media,
		Visibility:    params.Visibility,
		Logger:        params.Logger,
	})
}

func newDesktop(params desktopParams, settings *configuration.Settings) *desktopwails.Desktop {
	desktop := desktopwails.New(&desktopwails.Dependencies{
		Configuration: desktopConfiguration(settings),
		App:           params.App,
		Handles:       params.Handles,
		Windows:       params.Windows,
		Settings:      params.Settings,
		Visibility:    params.Visibility,
		Startup:       params.Startup,
		Reminders:     params.Reminders,
		Notifications: params.Notifications,
		Assets:        params.Assets,
	})
	params.Lifecycle.Append(desktopLifecycle(desktop, params.Runtime, params.Requests))

	return desktop
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

// NewBridge supplies graph control after construction, before running the native event loop.
func NewBridge(graph Lifecycle, runtime *lifetime.Runtime) *Bridge {
	var bridge Bridge

	bridge.graph = graph
	bridge.runtime = runtime

	return &bridge
}

// ServiceStartup starts the graph with a bounded context and an independent worker lifetime.
func (bridge *Bridge) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	bridge.start.Do(func() {
		bridge.runtime.Attach(ctx)

		startup, cancel := context.WithTimeout(ctx, lifecycleTimeout)

		defer cancel()

		bridge.startErr = isolate(bridge.graph.Start(startup))
		if bridge.startErr != nil {
			bridge.runtime.Cancel()
		}
	})

	return bridge.startErr
}

// ServiceShutdown releases the graph once, including after an unsuccessful Run.
func (bridge *Bridge) ServiceShutdown() error {
	bridge.stop.Do(func() {
		bridge.runtime.Cancel()

		ctx, cancel := context.WithTimeout(context.Background(), lifecycleTimeout)

		defer cancel()

		bridge.stopErr = isolate(bridge.graph.Stop(ctx))
	})

	return bridge.stopErr
}

func isolate(err error) error {
	if err != nil {
		return &lifecycleError{message: err.Error()}
	}

	return nil
}

func (failure *lifecycleError) Error() string { return failure.message }

func desktopConfiguration(settings *configuration.Settings) *desktopwails.Configuration {
	return &desktopwails.Configuration{
		Layout: settings.Layout, ApplicationID: settings.ApplicationID,
		LogErrorKey: settings.LogErrorKey, ShowReviews: settings.ShowReviews,
	}
}

func newWindows(app *application.App, settings *configuration.Settings) companionwails.WindowSet {
	return desktopwails.NewWindows(app, desktopConfiguration(settings))
}
