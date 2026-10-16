// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"fmt"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"raven/internal/reminders"
)

func New(native *notifications.NotificationService) reminders.Notifier {
	return notifier{native: native}
}

func (n notifier) CheckNotificationAuthorization() (bool, error) {
	return n.native.CheckNotificationAuthorization()
}

func (n notifier) Send(count int) error {
	var sound *notifications.NotificationSound

	if runtime.GOOS == "darwin" {
		sound = &notifications.NotificationSound{Name: "raven.wav"}
	}

	return n.native.SendNotification(
		notifications.NotificationOptions{
			Sound:    sound,
			ID:       "raven-due",
			Title:    "Time to review",
			Body:     fmt.Sprintf("%d items are ready to review in Raven.", count),
			ThreadID: "raven-reviews",
		},
	)
}

func (n notifier) RequestAuthorization() error {
	_, err := n.native.RequestNotificationAuthorization()

	return err
}
