// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// NewRuntime constructs an optional notification runtime without starting it.
func NewRuntime(native NativeRuntime, logger *slog.Logger, logErrorKey string) *Runtime {
	var runtime Runtime

	runtime.Notifier = New(native)
	runtime.native = native
	runtime.logger = logger
	runtime.logErrorKey = logErrorKey

	return &runtime
}

// Start initializes notifications without making native availability fatal.
func (runtime *Runtime) Start(ctx context.Context) {
	var options application.ServiceOptions

	err := runtime.native.ServiceStartup(ctx, options)
	if err != nil {
		runtime.logger.WarnContext(
			ctx,
			"desktop notifications unavailable",
			//nolint:sloglint // Log field names come from validated embedded configuration.
			slog.Any(runtime.logErrorKey, err),
		)

		return
	}

	runtime.available.Store(true)
}

// Available reports whether native initialization succeeded.
func (runtime *Runtime) Available() bool { return runtime.available.Load() }

// Stop releases only a successfully initialized notification service.
func (runtime *Runtime) Stop() error {
	if !runtime.available.Swap(false) {
		return nil
	}

	err := runtime.native.ServiceShutdown()
	if err != nil {
		return fmt.Errorf("stop notifications: %w", err)
	}

	return nil
}

// OnReview opens reviews only for a successful default notification action.
func (runtime *Runtime) OnReview(show func()) {
	runtime.native.OnNotificationResponse(func(result notifications.NotificationResult) {
		if result.Error != nil || !runtime.Available() {
			return
		}

		if result.Response.ActionIdentifier == notifications.DefaultActionIdentifier {
			show()
		}
	})
}

// New constructs the adapter with its required runtime dependencies.
func New(native NativeNotifications) Notifier {
	return Notifier{native: native}
}

// CheckNotificationAuthorization reports whether native notifications are authorized.
func (notifier Notifier) CheckNotificationAuthorization() (bool, error) {
	result, forwardErr := notifier.native.CheckNotificationAuthorization()
	if forwardErr != nil {
		return result, fmt.Errorf("check notification authorization: %w", forwardErr)
	}

	return result, nil
}

// Send delivers a reminder for the supplied number of due items.
func (notifier Notifier) Send(count int) error {
	err := notifier.native.SendNotification(reminderOptions(count))
	if err != nil {
		return fmt.Errorf("send reminder: %w", err)
	}

	return nil
}

func reminderOptions(count int) notifications.NotificationOptions {
	var options notifications.NotificationOptions

	options.Sound = reminderSound()
	options.ID = "raven-due"
	options.Title = "Time to review"
	options.Body = fmt.Sprintf("%d items are ready to review in Raven.", count)
	options.ThreadID = "raven-reviews"

	return options
}

// RequestAuthorization requests native notification authorization.
func (notifier Notifier) RequestAuthorization() (bool, error) {
	allowed, err := notifier.native.RequestNotificationAuthorization()
	if err != nil {
		return false, fmt.Errorf("request authorization: %w", err)
	}

	return allowed, nil
}
