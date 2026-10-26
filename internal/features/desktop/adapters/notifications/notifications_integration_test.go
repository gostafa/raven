package notifications_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	nativeadapter "raven/internal/features/desktop/adapters/notifications"
)

type native struct {
	options                   notifications.NotificationOptions
	checkFailure, sendFailure error
	allowed                   bool
	checks                    int
}

func (n *native) CheckNotificationAuthorization() (bool, error) {
	n.checks++
	return n.allowed, n.checkFailure
}

func (n *native) SendNotification(options notifications.NotificationOptions) error {
	n.options = options
	return n.sendFailure
}

func TestNativeMessagesRespectLifecycleAndAuthorization(t *testing.T) {
	n := &native{}
	available := false
	notifier := nativeadapter.New(n, func() bool { return available })
	if err := notifier.Notify("Raven", "Before startup"); err != nil || n.checks != 0 {
		t.Fatalf("unavailable runtime used: %v", err)
	}
	available = true
	if err := notifier.Notify("Raven", "Denied"); err != nil || n.options.ID != "" {
		t.Fatalf("denied notification delivered: %v", err)
	}
	n.allowed = true
	if err := notifier.Notify("Raven", "تم نسخ بطاقتين"); err != nil {
		t.Fatal(err)
	}
	first := n.options
	if first.Title != "Raven" || first.Body != "تم نسخ بطاقتين" ||
		!strings.HasPrefix(first.ID, "raven-notice-") || first.ThreadID != "raven-notices" ||
		first.Sound == nil || !first.Sound.Silent {
		t.Fatalf("incorrect notification: %+v", first)
	}
	if err := notifier.Notify("Raven", "Another message"); err != nil || n.options.ID == first.ID {
		t.Fatalf("notification replaced an earlier message: %v", err)
	}
}

func TestNativeFailuresArePreserved(t *testing.T) {
	n := &native{allowed: true}
	notifier := nativeadapter.New(n, func() bool { return true })
	n.checkFailure = errors.New("authorization unavailable")
	if err := notifier.Notify("Raven", "Message"); !errors.Is(err, n.checkFailure) {
		t.Fatalf("authorization failure hidden: %v", err)
	}
	n.checkFailure = nil
	n.sendFailure = errors.New("delivery unavailable")
	if err := notifier.Notify("Raven", "Message"); !errors.Is(err, n.sendFailure) {
		t.Fatalf("delivery failure hidden: %v", err)
	}
}
