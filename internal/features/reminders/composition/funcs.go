// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"context"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"go.uber.org/fx"

	configuration "raven/internal/features/configuration/domain"
	reminderwails "raven/internal/features/reminders/adapters/wails"
	reminders "raven/internal/features/reminders/application"
)

func newNative() *notifications.NotificationService { return notifications.New() }

//nolint:contextcheck // Native notifications must outlive the bounded startup context.
func newNotifications(
	params notificationParams,
	settings configuration.Settings,
) *reminderwails.Runtime {
	runtime := reminderwails.NewRuntime(params.Native, params.Logger, settings.LogErrorKey())
	params.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			runtime.Start(params.Runtime.Context())

			return nil
		},
		OnStop: func(context.Context) error { return runtime.Stop() },
	})

	return runtime
}

func newWorker(params workerParams) *reminders.Worker {
	runtime := reminders.NewWorker(params.Source, params.Notifications, params.Callbacks)
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

func newCallbacks(params callbacksParams, settings configuration.Settings) reminders.Callbacks {
	return reminders.Callbacks{
		Publish: func() { params.Events.Emit(settings.SchedulesChanged()) },
		Warn: func(message string, err error) {
			params.Logger.WarnContext(params.Runtime.Context(), "review reminder unavailable",
				//nolint:sloglint // Log field names come from validated embedded configuration.
				slog.String(settings.LogOperationKey(), message),
				//nolint:sloglint // Log field names come from validated embedded configuration.
				slog.Any(settings.LogErrorKey(), err))
		},
	}
}
