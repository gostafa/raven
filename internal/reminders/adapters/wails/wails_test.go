package wails

import (
	"errors"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type fakeNotifications struct {
	failure   error
	options   notifications.NotificationOptions
	requested bool
}

func (native *fakeNotifications) CheckNotificationAuthorization() (bool, error) {
	return true, native.failure
}

func (native *fakeNotifications) SendNotification(options notifications.NotificationOptions) error {
	native.options = options
	return native.failure
}

func (native *fakeNotifications) RequestNotificationAuthorization() (bool, error) {
	native.requested = true
	return true, native.failure
}

func TestNotifierTranslatesReminderAndPreservesFailures(t *testing.T) {
	native := &fakeNotifications{}
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
	if err := notifier.RequestAuthorization(); err != nil || !native.requested {
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
	if err := notifier.RequestAuthorization(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
