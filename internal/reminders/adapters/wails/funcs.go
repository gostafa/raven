// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"fmt"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

func New(native *notifications.NotificationService) Notifier {
	return Notifier{native: native}
}

func (notifier Notifier) CheckNotificationAuthorization() (bool, error) {
	result, forwardErr := notifier.native.CheckNotificationAuthorization()
	if forwardErr != nil {
		return result, fmt.Errorf("check notification authorization: %w", forwardErr)
	}

	return result, nil
}

func (notifier Notifier) Send(count int) error {
	var sound *notifications.NotificationSound

	if runtime.GOOS == "darwin" {
		sound = &notifications.NotificationSound{Name: "raven.wav"}
	}

	forwardErr2 := notifier.native.SendNotification(
		notifications.NotificationOptions{
			Sound:    sound,
			ID:       "raven-due",
			Title:    "Time to review",
			Body:     fmt.Sprintf("%d items are ready to review in Raven.", count),
			ThreadID: "raven-reviews",
		},
	)
	if forwardErr2 != nil {
		return fmt.Errorf("send: %w", forwardErr2)
	}

	return nil
}

func (notifier Notifier) RequestAuthorization() error {
	_, err := notifier.native.RequestNotificationAuthorization()
	if err != nil {
		return fmt.Errorf("request authorization: %w", err)
	}

	return nil
}
