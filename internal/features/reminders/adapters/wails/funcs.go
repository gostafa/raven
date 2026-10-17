// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// NewRuntime constructs an optional notification runtime without starting it.
func NewRuntime(native NativeRuntime, logger *slog.Logger, logErrorKey string) *Runtime {
	available := new(atomic.Bool)
	operations := New(native).operations
	operations["start"] = func(ctx context.Context, _ int, _ func()) notificationResult {
		startNativeNotifications(ctx, native, notificationWarning(logger, logErrorKey, available))
		return notificationResult{allowed: available.Load()}
	}
	bindNotificationLifecycle(operations, native, available)
	return &Runtime{operations: operations}
}

func notificationWarning(
	logger *slog.Logger,
	key string,
	available *atomic.Bool,
) func(context.Context, error) {
	return func(ctx context.Context, err error) {
		if err == nil {
			available.Store(true)
			return
		}
		logger.WarnContext(ctx, "desktop notifications unavailable",
			//nolint:sloglint // Log field names come from validated embedded configuration.
			slog.Any(key, err))
	}
}

func startNativeNotifications(
	ctx context.Context,
	native NativeRuntime,
	report func(context.Context, error),
) {
	report(ctx, native.ServiceStartup(ctx, application.ServiceOptions{}))
}

func bindNotificationLifecycle(
	operations map[string]func(context.Context, int, func()) notificationResult,
	native NativeRuntime,
	available *atomic.Bool,
) {
	operations["available"] = func(context.Context, int, func()) notificationResult {
		return notificationResult{allowed: available.Load()}
	}
	operations["stop"] = func(context.Context, int, func()) notificationResult {
		return notificationResult{failure: stopNativeNotifications(native, available)}
	}
	operations["review"] = func(_ context.Context, _ int, show func()) notificationResult {
		native.OnNotificationResponse(func(result notifications.NotificationResult) {
			notificationResponse(result, available, show)
		})
		return notificationResult{}
	}
}

func stopNativeNotifications(native NativeRuntime, available *atomic.Bool) error {
	if !available.Swap(false) {
		return nil
	}
	err := native.ServiceShutdown()
	if err != nil {
		return fmt.Errorf("stop notifications: %w", err)
	}
	return nil
}

func notificationResponse(
	result notifications.NotificationResult,
	available *atomic.Bool,
	show func(),
) {
	if result.Error != nil || !available.Load() {
		return
	}
	if result.Response.ActionIdentifier == notifications.DefaultActionIdentifier {
		show()
	}
}

// Start initializes notifications without making native availability fatal.
func (runtime *Runtime) Start(ctx context.Context) { runtime.operations["start"](ctx, 0, nil) }

// Available reports whether native initialization succeeded.
func (runtime *Runtime) Available() bool {
	return runtime.operations["available"](context.Background(), 0, nil).allowed
}

// Stop releases only a successfully initialized notification service.
func (runtime *Runtime) Stop() error {
	return runtime.operations["stop"](context.Background(), 0, nil).failure
}

// OnReview opens reviews only for a successful default notification action.
func (runtime *Runtime) OnReview(show func()) {
	runtime.operations["review"](context.Background(), 0, show)
}

// CheckNotificationAuthorization reports whether native notifications are authorized.
func (runtime *Runtime) CheckNotificationAuthorization() (bool, error) {
	return authorizationResult(runtime.operations["check"](context.Background(), 0, nil))
}

// RequestAuthorization requests native notification authorization.
func (runtime *Runtime) RequestAuthorization() (bool, error) {
	return authorizationResult(runtime.operations["request"](context.Background(), 0, nil))
}

// Send delivers a reminder for the supplied number of due items.
func (runtime *Runtime) Send(count int) error {
	return runtime.operations["send"](context.Background(), count, nil).failure
}

// New constructs the adapter with its required runtime dependencies.
func New(native NativeNotifications) Notifier {
	return Notifier{operations: map[string]func(context.Context, int, func()) notificationResult{
		"check":   func(context.Context, int, func()) notificationResult { return checkAuthorization(native) },
		"request": func(context.Context, int, func()) notificationResult { return requestAuthorization(native) },
		"send": func(_ context.Context, count int, _ func()) notificationResult {
			return notificationResult{failure: sendReminder(native, count)}
		},
	}}
}

// CheckNotificationAuthorization reports whether native notifications are authorized.
func (notifier Notifier) CheckNotificationAuthorization() (bool, error) {
	return authorizationResult(notifier.operations["check"](context.Background(), 0, nil))
}

// Send delivers a reminder for the supplied number of due items.
func (notifier Notifier) Send(count int) error {
	return notifier.operations["send"](context.Background(), count, nil).failure
}

// RequestAuthorization requests native notification authorization.
func (notifier Notifier) RequestAuthorization() (bool, error) {
	return authorizationResult(notifier.operations["request"](context.Background(), 0, nil))
}

func authorizationResult(result notificationResult) (bool, error) {
	return result.allowed, result.failure
}

func checkAuthorization(native NativeNotifications) notificationResult {
	allowed, err := native.CheckNotificationAuthorization()
	if err != nil {
		return notificationResult{
			allowed: allowed,
			failure: fmt.Errorf("check notification authorization: %w", err),
		}
	}
	return notificationResult{allowed: allowed}
}

func requestAuthorization(native NativeNotifications) notificationResult {
	allowed, err := native.RequestNotificationAuthorization()
	if err != nil {
		return notificationResult{failure: fmt.Errorf("request authorization: %w", err)}
	}
	return notificationResult{allowed: allowed}
}

func sendReminder(native NativeNotifications, count int) error {
	err := native.SendNotification(reminderOptions(count))
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
