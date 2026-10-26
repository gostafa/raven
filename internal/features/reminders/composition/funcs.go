// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"context"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"go.uber.org/fx"

	configuration "raven/internal/features/configuration/domain"
	desktopnotifications "raven/internal/features/desktop/adapters/notifications"
	reminderwails "raven/internal/features/reminders/adapters/wails"
	reminders "raven/internal/features/reminders/application"
	"raven/internal/shared/lifetime"
)

func newNative() *notifications.NotificationService { return notifications.New() }

func newApplicationNotifications(
	native reminderwails.NativeRuntime,
	runtime *reminderwails.Runtime,
) *desktopnotifications.Notifier {
	return desktopnotifications.New(native, runtime.Available)
}

//nolint:contextcheck // Native notifications must outlive the bounded startup context.
func newNotifications(
	params notificationParams,
) *reminderwails.Runtime {
	runtime := newNotificationRuntime(params)
	params.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go runtime.Start(params.Runtime.Context())

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

func newSound(runtime *lifetime.Runtime) reminderwails.SoundPlayer {
	return reminderwails.NewSound(runtime.Context())
}

func newNotificationRuntime(
	params notificationParams,
) *reminderwails.Runtime {
	options := reminderwails.NewRuntimeOptions(func(ctx context.Context, err error) {
		params.Logger.WarnContext(
			ctx,
			"desktop notifications unavailable",
			slog.Any(notificationErrorKey, err),
		)
	}, params.Messages)

	options.Logger = params.Logger

	return reminderwails.NewLocalizedRuntime(
		params.Native,
		params.Sound,
		options,
	)
}
