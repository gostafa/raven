// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"context"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"go.uber.org/fx"

	reminderwails "raven/internal/features/reminders/adapters/wails"
	"raven/internal/features/reminders/adapters/worker"
	reminders "raven/internal/features/reminders/application"
)

func newNative() reminderwails.NativeRuntime { return notifications.New() }

func newNotifications(params notificationParams) *reminderwails.Runtime {
	runtime := reminderwails.NewRuntime(params.Native, params.Logger)
	params.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error { return runtime.Start(params.Runtime.Context()) },
		OnStop:  func(context.Context) error { return runtime.Stop() },
	})

	return runtime
}

func newWorker(params workerParams) *worker.Worker {
	runtime := worker.New(params.Source, params.Notifications, params.Callbacks)
	params.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			runtime.Enable()
			return nil
		},

		OnStop: func(context.Context) error {
			runtime.Stop()
			return nil
		},
	})

	return runtime
}

func newCallbacks(params callbacksParams) reminders.Callbacks {
	return reminders.Callbacks{
		Publish: func() { params.Events.Emit("raven:schedules-changed") },
		Warn: func(message string, err error) {
			params.Logger.WarnContext(params.Runtime.Context(), "review reminder unavailable",
				slog.String("operation", message), slog.Any("error", err))
		},
	}
}
