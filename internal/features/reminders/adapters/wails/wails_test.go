package wails

import (
	"context"
	"errors"
	"io"
	"log/slog"
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
		runtime := NewRuntime(native, slog.New(slog.NewTextHandler(io.Discard, nil)), "error")
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
	runtime := NewRuntime(native, slog.Default(), "error")
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
	notifier := New(native)
	if allowed, err := notifier.CheckNotificationAuthorization(); err != nil || !allowed {
		t.Fatalf("authorization=%v error=%v", allowed, err)
	}
	if err := notifier.Send(7); err != nil {
		t.Fatal(err)
	}
	if native.options.ID != "raven-due" || native.options.ThreadID != "raven-reviews" ||
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
	allowed, err := New(native).RequestAuthorization()
	if err != nil || allowed || !native.requested {
		t.Fatalf(
			"denied permission: allowed=%v requested=%v error=%v",
			allowed,
			native.requested,
			err,
		)
	}
}
