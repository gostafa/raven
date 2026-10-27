// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"raven/internal/features/reminders/ports"
	resilience "raven/internal/shared/resilience/adapters/retry"
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
	state := newNotificationLifecycle(native, options)
	operations := New(native, sound, options.Messages).operations

	operations["start"] = func(ctx context.Context, _ int, _ func()) notificationResult {
		return startManagedNotifications(ctx, state)
	}
	bindManagedNotifications(operations, state)

	return &Runtime{operations: operations}
}

func newNotificationLifecycle(
	native NativeRuntime,
	options *ports.RuntimeOptions,
) *notificationLifecycle {
	owner, cancel := context.WithCancel(context.Background())
	state := new(notificationLifecycle)

	state.native, state.owner, state.cancel = native, func() context.Context { return owner }, cancel
	state.available, state.listeners, state.warn = new(
		atomic.Bool,
	), make(
		map[uint]func(),
	), options.Warn
	state.resource = newNotificationResource(state, options)

	return state
}

func newNotificationResource(
	state *notificationLifecycle,
	options *ports.RuntimeOptions,
) *resilience.Initialization[bool] {
	return resilience.NewInitialization(
		state.owner(),
		func(ctx context.Context) (bool, error) { return startNativeAttempt(ctx, state) },
		&resilience.Options[bool]{
			Name:      "notifications",
			Logger:    options.Logger,
			Release:   func(bool) error { return state.native.ServiceShutdown() },
			Transient: notificationTransient,
		},
	)
}

//nolint:contextcheck // Native workers need the owner lifetime, independent of the initialization deadline.
func startNativeAttempt(ctx context.Context, state *notificationLifecycle) (bool, error) {
	var options application.ServiceOptions

	err := errors.Join(state.native.ServiceStartup(state.owner(), options), ctx.Err())
	if err != nil {
		return false, errors.Join(cleanupNativeFailure(state.native, err))
	}

	return true, nil
}

func cleanupNativeFailure(native NativeRuntime, err error) error {
	cleanup := native.ServiceShutdown()
	if cleanup != nil {
		return errors.Join(err, cleanup, errNotificationCleanup)
	}

	return errors.Join(err)
}

func startManagedNotifications(
	ctx context.Context,
	state *notificationLifecycle,
) notificationResult {
	detach := context.AfterFunc(ctx, state.cancel)

	allowed, err := state.resource.Get(ctx)
	if err != nil {
		detach()
		handleNotificationStartupFailure(ctx, state, err)
	}

	publishNotificationAvailability(state, allowed)

	return notificationResult{allowed: state.available.Load(), failure: nil, unsubscribe: nil}
}

func handleNotificationStartupFailure(
	ctx context.Context,
	state *notificationLifecycle,
	err error,
) {
	state.cancel()

	err = errors.Join(err, state.resource.Close())
	recordNotificationStartupCleanupFailure(state, err)
	state.warn(ctx, err)
}

func publishNotificationAvailability(state *notificationLifecycle, allowed bool) {
	state.mu.Lock()
	state.available.Store(allowed && !state.stopped)

	callbacks := notificationListeners(state)
	state.mu.Unlock()

	for index := range callbacks {
		callbacks[index]()
	}
}

func closeManagedNotifications(state *notificationLifecycle) error {
	state.mu.Lock()

	state.stopped = true
	state.available.Store(false)
	state.mu.Unlock()
	state.cancel()

	state.mu.Lock()

	startupCleanupError := state.startupCleanupError
	state.mu.Unlock()

	return errors.Join(startupCleanupError, state.resource.Close())
}

func bindManagedNotifications(
	operations map[string]func(context.Context, int, func()) notificationResult,
	state *notificationLifecycle,
) {
	operations["available"] = func(context.Context, int, func()) notificationResult {
		return notificationResult{allowed: state.available.Load(), failure: nil, unsubscribe: nil}
	}
	operations["stop"] = func(context.Context, int, func()) notificationResult {
		return notificationResult{
			allowed:     false,
			failure:     closeManagedNotifications(state),
			unsubscribe: nil,
		}
	}
	bindNotificationSubscription(operations, state)
	bindNotificationResponses(operations, state)
}

func bindNotificationSubscription(
	operations map[string]func(context.Context, int, func()) notificationResult,
	state *notificationLifecycle,
) {
	operations["subscribe"] = func(_ context.Context, _ int, changed func()) notificationResult {
		return notificationResult{
			allowed:     false,
			failure:     nil,
			unsubscribe: observeNotifications(state, changed),
		}
	}
}

func bindNotificationResponses(
	operations map[string]func(context.Context, int, func()) notificationResult,
	state *notificationLifecycle,
) {
	operations["review"] = func(_ context.Context, _ int, show func()) notificationResult {
		state.native.OnNotificationResponse(
			func(result notifications.NotificationResult) { notificationResponse(&result, state.available, show) },
		)

		return notificationResult{allowed: false, failure: nil, unsubscribe: nil}
	}
}

func observeNotifications(state *notificationLifecycle, changed func()) func() {
	state.mu.Lock()

	state.next++

	key := state.next

	state.listeners[key] = changed
	state.mu.Unlock()

	return func() { state.mu.Lock(); delete(state.listeners, key); state.mu.Unlock() }
}

func notificationListeners(state *notificationLifecycle) []func() {
	callbacks := make([]func(), zero, len(state.listeners))
	for key := range state.listeners {
		callbacks = append(callbacks, state.listeners[key])
	}

	return callbacks
}

// SubscribeAvailability observes native startup completion without coupling the worker to Wails.
func (runtime *Runtime) SubscribeAvailability(changed func()) func() {
	return runtime.operations["subscribe"](context.Background(), zero, changed).unsubscribe
}

func notificationResponse(
	result *notifications.NotificationResult,
	available *atomic.Bool,
	show func(),
) {
	if result.Error != nil || !available.Load() {
		return
	}

	if strings.HasPrefix(result.Response.ID, "raven-notice-") {
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
				allowed:     false,
				unsubscribe: nil,
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

	return authorizationResult(allowed, err, "check notification authorization")
}

func requestAuthorization(native NativeNotifications) notificationResult {
	allowed, err := native.RequestNotificationAuthorization()
	if err != nil {
		allowed = false
	}

	return authorizationResult(allowed, err, "request authorization")
}

func authorizationResult(allowed bool, err error, operation string) notificationResult {
	result := notificationResult{allowed: allowed, failure: nil, unsubscribe: nil}

	if err != nil {
		result.failure = fmt.Errorf("%s: %w", operation, err)
	}

	return result
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
	return &ports.RuntimeOptions{Warn: warn, Messages: messages, Logger: nil}
}
