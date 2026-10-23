package wails

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type fakeRuntime struct {
	fakeNotifications
	callback func(notifications.NotificationResult)
	started  context.Context
	stops    int
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
		if runtime.Available() == failStart || native.started != t.Context() {
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
		if runtime.Available() || native.stops != opened {
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
	options        notifications.NotificationOptions
	requested      bool
	requestAllowed bool
}

func (native *fakeNotifications) CheckNotificationAuthorization() (bool, error) {
	return native.requestAllowed, native.failure
}

func (native *fakeNotifications) SendNotification(options notifications.NotificationOptions) error {
	native.options = options
	return native.failure
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
