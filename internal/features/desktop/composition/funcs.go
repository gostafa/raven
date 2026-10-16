// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"

	desktopwails "raven/internal/features/desktop/adapters/wails"
	"raven/internal/shared/lifetime"
)

func newApplication(params appParams) *application.App {
	return desktopwails.NewApplication(desktopwails.ApplicationDependencies{
		Assets:     params.Assets,
		Media:      params.Media,
		Visibility: params.Visibility,
		Logger:     params.Logger,
	})
}

func newDesktop(params desktopParams) *desktopwails.Desktop {
	desktop := desktopwails.New(desktopwails.Dependencies{
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
	done := make(chan struct{})

	params.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() { defer close(done); desktop.Open(params.Runtime.Context(), params.Requests.Pending()) }()

			return nil
		},

		OnStop: func(context.Context) error {
			desktop.Dispose()
			params.Runtime.Cancel()
			<-done
			return nil
		},
	})

	return desktop
}

func configureDesktop(desktop *desktopwails.Desktop) { desktop.Configure() }

// NewBridge supplies graph control after construction, before running the native event loop.
func NewBridge(graph Lifecycle, runtime *lifetime.Runtime) *Bridge {
	return &Bridge{graph: graph, runtime: runtime}
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
