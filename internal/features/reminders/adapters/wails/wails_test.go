package wails

import (
	"context"
	"errors"
	"net"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	desktopnotifications "raven/internal/features/desktop/adapters/notifications"
)

type fakeRuntime struct {
	fakeNotifications
	callback func(notifications.NotificationResult)
	started  context.Context
	stops    int
}

type recoveringNative struct {
	fakeRuntime
	attempts       atomic.Int32
	cleanups       atomic.Int32
	cleanupFailure error
}

func (native *recoveringNative) ServiceStartup(context.Context, application.ServiceOptions) error {
	if native.attempts.Add(1) == 1 {
		return syscall.EAGAIN
	}
	return nil
}

func (native *recoveringNative) ServiceShutdown() error {
	native.cleanups.Add(1)
	return native.cleanupFailure
}

func TestNotificationStartupRecoveryAndCleanup(t *testing.T) {
	if notificationTransient(errors.New("unknown")) {
		t.Fatal("unknown failure retried")
	}
	if notificationTransient(
		&net.OpError{Err: os.ErrDeadlineExceeded},
	) != (runtime.GOOS != "windows") {
		t.Fatal("incorrect timeout classification")
	}
	if runtime.GOOS == "windows" {
		var warnings atomic.Int32
		failed := &recoveringNative{cleanupFailure: errors.New("cleanup failed")}
		service := NewRuntime(failed, &fakeSound{}, func(context.Context, error) {
			warnings.Add(1)
		})
		service.Start(t.Context())
		if failed.attempts.Load() != 1 || failed.cleanups.Load() != 1 ||
			service.Available() || warnings.Load() != 1 {
			t.Fatal("Windows startup failure was not reported")
		}
		if err := service.Stop(); err == nil {
			t.Fatal("Windows cleanup failure was hidden")
		}
		return
	}
	native := &recoveringNative{}
	service := NewRuntime(native, &fakeSound{}, func(context.Context, error) {})
	var changes atomic.Int32
	unsubscribe := service.SubscribeAvailability(func() { changes.Add(1) })
	service.Start(t.Context())
	if !service.Available() || native.attempts.Load() != 2 || native.cleanups.Load() != 1 ||
		changes.Load() != 1 {
		t.Fatal("startup did not recover")
	}
	unsubscribe()
	if err := service.Stop(); err != nil {
		t.Fatal(err)
	}
	if native.cleanups.Load() != 2 || service.Available() {
		t.Fatal("resource retained after shutdown")
	}
	failed := &recoveringNative{cleanupFailure: errors.New("cleanup failed")}
	service = NewRuntime(failed, &fakeSound{}, func(context.Context, error) {})
	service.Start(t.Context())
	if failed.attempts.Load() != 1 || service.Available() {
		t.Fatal("retried after failed cleanup")
	}
	if err := service.Stop(); err != nil {
		t.Fatal(err)
	}
}

type fakeSound struct {
	plays   int
	failure error
}

func (sound *fakeSound) Play() error {
	sound.plays++
	return sound.failure
}

func TestReminderPlaysRavenAudioOncePerDelivery(t *testing.T) {
	native := &fakeNotifications{}
	sound := &fakeSound{}
	notifier := New(native, sound)
	if err := notifier.Send(3); err != nil {
		t.Fatal(err)
	}
	firstID := native.options.ID
	if err := notifier.Send(1); err != nil {
		t.Fatal(err)
	}
	if sound.plays != 2 || native.options.ID == firstID {
		t.Fatal("lost or replaced card sound")
	}
	sound.failure = errors.New("audio unavailable")
	if err := notifier.Send(1); !errors.Is(err, sound.failure) {
		t.Fatalf("playback failure hidden: %v", err)
	}
}

func (native *fakeRuntime) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	native.started = ctx
	return native.failure
}

func (native *fakeRuntime) ServiceShutdown() error {
	native.stops++
	return native.failure
}

func (native *fakeRuntime) OnNotificationResponse(callback func(notifications.NotificationResult)) {
	native.callback = callback
}

func TestOptionalNotificationLifecycleAndReviewActions(t *testing.T) {
	for _, failStart := range []bool{false, true} {
		native := &fakeRuntime{}
		failure := errors.New("native failure")
		if failStart {
			native.failure = failure
		}
		runtime := NewRuntime(native, &fakeSound{}, func(context.Context, error) {})
		opened := 0
		runtime.OnReview(func() { opened++ })
		var response notifications.NotificationResult
		response.Response.ActionIdentifier = notifications.DefaultActionIdentifier
		native.callback(response)
		runtime.Start(t.Context())
		if runtime.Available() == failStart || native.started == nil ||
			native.started.Err() != nil && !failStart {
			t.Fatal("incorrect startup state or context")
		}
		native.callback(notifications.NotificationResult{Error: failure})
		native.callback(notifications.NotificationResult{})
		native.callback(response)
		if failStart {
			if opened != 0 {
				t.Fatal("unavailable runtime opened reviews")
			}
		} else if opened != 1 {
			t.Fatal("default action did not open reviews exactly once")
		}
		if err := runtime.Stop(); err != nil {
			t.Fatal(err)
		}
		if err := runtime.Stop(); err != nil {
			t.Fatal(err)
		}
		if runtime.Available() || native.stops != 1 {
			t.Fatal("incorrect shutdown state")
		}
	}
	native := &fakeRuntime{}
	runtime := NewRuntime(native, &fakeSound{}, func(context.Context, error) {})
	runtime.Start(t.Context())
	failure := errors.New("shutdown failure")
	native.failure = failure
	if err := runtime.Stop(); !errors.Is(err, failure) {
		t.Fatalf("shutdown error: %v", err)
	}
	if runtime.Available() {
		t.Fatal("failed shutdown retained availability")
	}
}

type fakeNotifications struct {
	failure        error
	sendFailure    error
	options        notifications.NotificationOptions
	requested      bool
	requestAllowed bool
}

func (native *fakeNotifications) CheckNotificationAuthorization() (bool, error) {
	return native.requestAllowed, native.failure
}

func (native *fakeNotifications) SendNotification(options notifications.NotificationOptions) error {
	native.options = options
	return errors.Join(native.failure, native.sendFailure)
}

func (native *fakeNotifications) RequestNotificationAuthorization() (bool, error) {
	native.requested = true
	return native.requestAllowed, native.failure
}

func TestNotifierTranslatesReminderAndPreservesFailures(t *testing.T) {
	native := &fakeNotifications{requestAllowed: true}
	notifier := New(native, &fakeSound{})
	if allowed, err := notifier.CheckNotificationAuthorization(); err != nil || !allowed {
		t.Fatalf("authorization=%v error=%v", allowed, err)
	}
	if err := notifier.Send(7); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(native.options.ID, "raven-due-") ||
		native.options.ThreadID != "raven-reviews" ||
		!strings.Contains(native.options.Body, "7 items") {
		t.Fatalf("lost reminder options: %+v", native.options)
	}
	if allowed, err := notifier.RequestAuthorization(); err != nil || !allowed ||
		!native.requested {
		t.Fatalf("request error=%v", err)
	}
	failure := errors.New("native notifications failed")
	native.failure = failure
	if _, err := notifier.CheckNotificationAuthorization(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := notifier.Send(1); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if allowed, err := notifier.RequestAuthorization(); !errors.Is(err, failure) || allowed {
		t.Fatal(err)
	}
}

func TestAuthorizationDenialIsReturnedWithoutAnError(t *testing.T) {
	native := &fakeNotifications{}
	allowed, err := New(native, &fakeSound{}).RequestAuthorization()
	if err != nil || allowed || !native.requested {
		t.Fatalf(
			"denied permission: allowed=%v requested=%v error=%v",
			allowed,
			native.requested,
			err,
		)
	}
}

func TestRuntimeSharesNotificationCapabilities(t *testing.T) {
	native := &fakeRuntime{}
	runtime := NewRuntime(native, &fakeSound{}, func(context.Context, error) {})
	if allowed, err := runtime.CheckNotificationAuthorization(); err != nil || allowed {
		t.Fatalf("authorization=%v error=%v", allowed, err)
	}
	if allowed, err := runtime.RequestAuthorization(); err != nil || allowed || !native.requested {
		t.Fatalf("request=%v error=%v", allowed, err)
	}
	if err := runtime.Send(3); err != nil || !strings.Contains(native.options.Body, "3 items") {
		t.Fatalf("notification=%+v error=%v", native.options, err)
	}
}

func TestPlayerReportsPlaybackResult(t *testing.T) {
	failure := errors.New("playback failed")
	player := SoundPlayer{play: func() error { return failure }}
	if err := player.Play(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}

type reminderMessages struct{}

func (reminderMessages) Text(key string, count int) string { return key + ":ar" }

func TestLocalizedReminderOptions(t *testing.T) {
	options := reminderOptions(3, reminderMessages{})
	if options.Title != "reminderTitle:ar" || options.Body != "reminderBody:ar" {
		t.Fatal(options)
	}
}

func TestApplicationNotificationsUseNativeDeliveryWithoutReviewActions(t *testing.T) {
	native := &fakeRuntime{}
	sound := &fakeSound{}
	runtime := NewRuntime(native, sound, func(context.Context, error) {})
	notices := desktopnotifications.New(native, runtime.Available)
	if err := notices.Notify("Raven", "Before startup"); err != nil || native.options.ID != "" {
		t.Fatalf("unavailable delivery: %v", err)
	}
	runtime.Start(t.Context())
	if err := notices.Notify("Raven", "Permission denied"); err != nil || native.options.ID != "" {
		t.Fatalf("denied delivery: %v", err)
	}
	native.requestAllowed = true
	if err := notices.Notify("Raven", "تم نسخ بطاقتين"); err != nil {
		t.Fatal(err)
	}
	options := native.options
	if options.Title != "Raven" || options.Body != "تم نسخ بطاقتين" ||
		options.ThreadID != "raven-notices" || !strings.HasPrefix(options.ID, "raven-notice-") ||
		options.Sound == nil || !options.Sound.Silent || sound.plays != 0 {
		t.Fatalf("incorrect application notification: %+v", options)
	}
	opened := 0
	runtime.OnReview(func() { opened++ })
	var response notifications.NotificationResult
	response.Response.ID = options.ID
	response.Response.ActionIdentifier = notifications.DefaultActionIdentifier
	native.callback(response)
	if opened != 0 {
		t.Fatal("application message opened a review")
	}
	if err := notices.Notify(
		"Raven",
		"Another message",
	); err != nil ||
		native.options.ID == options.ID {
		t.Fatalf("message replaced an earlier notification: %v", err)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatal(err)
	}
	previous := native.options.ID
	if err := notices.Notify(
		"Raven",
		"After shutdown",
	); err != nil ||
		native.options.ID != previous {
		t.Fatalf("stopped runtime delivered a notification: %v", err)
	}
}

func TestApplicationNotificationFailuresAreReturned(t *testing.T) {
	native := &fakeRuntime{}
	runtime := NewRuntime(native, &fakeSound{}, func(context.Context, error) {})
	notices := desktopnotifications.New(native, runtime.Available)
	runtime.Start(t.Context())
	native.failure = errors.New("native delivery unavailable")
	if err := notices.Notify("Raven", "Message"); !errors.Is(err, native.failure) {
		t.Fatalf("native failure hidden: %v", err)
	}
	native.failure = nil
	native.requestAllowed = true
	native.sendFailure = errors.New("delivery failed")
	if err := notices.Notify("Raven", "Message"); !errors.Is(err, native.sendFailure) {
		t.Fatalf("delivery failure hidden: %v", err)
	}
}
