// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"raven/internal/features/reminders/ports"
)

// NewRuntime constructs an optional notification runtime without starting it.
func NewRuntime(
	native NativeRuntime,
	sound ports.Sound,
	warn func(context.Context, error),
) *Runtime {
	return NewLocalizedRuntime(native, sound, NewRuntimeOptions(warn, nil))
}

// NewLocalizedRuntime constructs notification delivery with live localized messages.
func NewLocalizedRuntime(
	native NativeRuntime,
	sound ports.Sound,
	options *ports.RuntimeOptions,
) *Runtime {
	available := new(atomic.Bool)
	operations := New(native, sound, options.Messages).operations

	operations["start"] = func(ctx context.Context, _ int, _ func()) notificationResult {
		startNativeNotifications(
			ctx,
			native,
			notificationWarning(options.Warn, available),
		)

		return notificationResult{allowed: available.Load(), failure: nil}
	}
	bindNotificationLifecycle(operations, native, available)

	return &Runtime{operations: operations}
}

func notificationWarning(
	warn func(context.Context, error),
	available *atomic.Bool,
) func(context.Context, error) {
	return func(ctx context.Context, err error) {
		if err == nil {
			available.Store(true)

			return
		}

		warn(ctx, err)
	}
}

func startNativeNotifications(
	ctx context.Context,
	native NativeRuntime,
	report func(context.Context, error),
) {
	var options application.ServiceOptions

	report(ctx, native.ServiceStartup(ctx, options))
}

func bindNotificationLifecycle(
	operations map[string]func(context.Context, int, func()) notificationResult,
	native NativeRuntime,
	available *atomic.Bool,
) {
	operations["available"] = func(context.Context, int, func()) notificationResult {
		return notificationResult{allowed: available.Load(), failure: nil}
	}
	operations["stop"] = func(context.Context, int, func()) notificationResult {
		return notificationResult{
			allowed: false,
			failure: stopNativeNotifications(native, available),
		}
	}
	operations["review"] = func(_ context.Context, _ int, show func()) notificationResult {
		native.OnNotificationResponse(func(result notifications.NotificationResult) {
			notificationResponse(&result, available, show)
		})

		return notificationResult{allowed: false, failure: nil}
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
	result *notifications.NotificationResult,
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
func (runtime *Runtime) Start(ctx context.Context) { runtime.operations["start"](ctx, zero, nil) }

// Available reports whether native initialization succeeded.
func (runtime *Runtime) Available() bool {
	return runtime.operations["available"](context.Background(), zero, nil).allowed
}

// Stop releases only a successfully initialized notification service.
func (runtime *Runtime) Stop() error {
	return runtime.operations["stop"](context.Background(), zero, nil).failure
}

// OnReview opens reviews only for a successful default notification action.
func (runtime *Runtime) OnReview(show func()) {
	runtime.operations["review"](context.Background(), zero, show)
}

// CheckNotificationAuthorization reports whether native notifications are authorized.
func (runtime *Runtime) CheckNotificationAuthorization() (bool, error) {
	result := runtime.operations["check"](context.Background(), zero, nil)

	return result.allowed, result.failure
}

// RequestAuthorization requests native notification authorization.
func (runtime *Runtime) RequestAuthorization() (bool, error) {
	result := runtime.operations["request"](context.Background(), zero, nil)

	return result.allowed, result.failure
}

// Send delivers a reminder for the supplied number of due items.
func (runtime *Runtime) Send(count int) error {
	return runtime.operations["send"](context.Background(), count, nil).failure
}

// New constructs the adapter with its required runtime dependencies.
func New(native NativeNotifications, sound ports.Sound, messages ...ports.Messages) Notifier {
	return Notifier{operations: map[string]func(context.Context, int, func()) notificationResult{
		"check":   func(context.Context, int, func()) notificationResult { return checkAuthorization(native) },
		"request": func(context.Context, int, func()) notificationResult { return requestAuthorization(native) },
		"send": func(_ context.Context, count int, _ func()) notificationResult {
			return notificationResult{
				allowed: false,
				failure: sendReminder(
					native,
					sound,
					&reminderDelivery{count: count, messages: messages},
				),
			}
		},
	}}
}

// CheckNotificationAuthorization reports whether native notifications are authorized.
func (notifier Notifier) CheckNotificationAuthorization() (bool, error) {
	result := notifier.operations["check"](context.Background(), zero, nil)

	return result.allowed, result.failure
}

// Send delivers a reminder for the supplied number of due items.
func (notifier Notifier) Send(count int) error {
	return notifier.operations["send"](context.Background(), count, nil).failure
}

// RequestAuthorization requests native notification authorization.
func (notifier Notifier) RequestAuthorization() (bool, error) {
	result := notifier.operations["request"](context.Background(), zero, nil)

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

	return notificationResult{allowed: allowed, failure: nil}
}

func requestAuthorization(native NativeNotifications) notificationResult {
	allowed, err := native.RequestNotificationAuthorization()
	if err != nil {
		return notificationResult{
			allowed: false,
			failure: fmt.Errorf("request authorization: %w", err),
		}
	}

	return notificationResult{allowed: allowed, failure: nil}
}

func sendReminder(native NativeNotifications, sound ports.Sound, delivery *reminderDelivery) error {
	err := sound.Play()
	if err != nil {
		return fmt.Errorf("play reminder sound: %w", err)
	}

	err = native.SendNotification(reminderOptions(delivery.count, delivery.messages...))
	if err != nil {
		return fmt.Errorf("send reminder: %w", err)
	}

	return nil
}

func reminderOptions(count int, messages ...ports.Messages) notifications.NotificationOptions {
	var options notifications.NotificationOptions

	options.Sound = reminderSound()
	options.ID = "raven-due-" + rand.Text()
	options.Title = "Time to review"
	options.Body = fmt.Sprintf("%d items are ready to review in Raven.", count)

	if len(messages) > zero && messages[zero] != nil {
		options.Title = messages[zero].Text("reminderTitle", count)
		options.Body = messages[zero].Text("reminderBody", count)
	}

	options.ThreadID = "raven-reviews"

	return options
}

// NewSound constructs a player bound to the cancellable application runtime.
func NewSound(ctx context.Context) SoundPlayer {
	return SoundPlayer{play: func() error { return playReminder(ctx, os.Executable) }}
}

// Play waits for one playback to finish so simultaneous cards do not overlap.
func (player SoundPlayer) Play() error { return errors.Join(player.play()) }

// NewRuntimeOptions combines localized messages and notification diagnostics.
func NewRuntimeOptions(
	warn func(context.Context, error),
	messages ports.Messages,
) *ports.RuntimeOptions {
	return &ports.RuntimeOptions{Warn: warn, Messages: messages}
}
