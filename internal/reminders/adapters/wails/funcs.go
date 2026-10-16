// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// New constructs the adapter with its required runtime dependencies.
func New(native NativeNotifications) Notifier {
	return Notifier{native: native}
}

// CheckNotificationAuthorization reports whether native notifications are authorized.
func (notifier Notifier) CheckNotificationAuthorization() (bool, error) {
	result, forwardErr := notifier.native.CheckNotificationAuthorization()
	if forwardErr != nil {
		return result, fmt.Errorf("check notification authorization: %w", forwardErr)
	}

	return result, nil
}

// Send delivers a reminder for the supplied number of due items.
func (notifier Notifier) Send(count int) error {
	err := notifier.native.SendNotification(reminderOptions(count))
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

// RequestAuthorization requests native notification authorization.
func (notifier Notifier) RequestAuthorization() error {
	_, err := notifier.native.RequestNotificationAuthorization()
	if err != nil {
		return fmt.Errorf("request authorization: %w", err)
	}

	return nil
}
