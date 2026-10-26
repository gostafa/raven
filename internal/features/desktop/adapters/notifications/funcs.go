// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package notifications

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// New observes the availability of the shared native runtime without starting it.
func New(native Native, available func() bool) *Notifier {
	return &Notifier{send: func(title, body string) error {
		if !available() {
			return nil
		}

		return send(native, &message{title: title, body: body})
	}}
}

// Notify delivers an application message using the managed native service.
func (notifier *Notifier) Notify(title, body string) error {
	return errors.Join(notifier.send(title, body))
}

func send(native Native, delivery *message) error {
	allowed, err := native.CheckNotificationAuthorization()
	if err != nil {
		return fmt.Errorf("check notification authorization: %w", err)
	}

	if !allowed {
		return nil
	}

	err = native.SendNotification(options(delivery))
	if err != nil {
		return fmt.Errorf("send application notification: %w", err)
	}

	return nil
}

func options(delivery *message) notifications.NotificationOptions {
	var result notifications.NotificationOptions

	result.ID = "raven-notice-" + rand.Text()
	result.Title, result.Body = delivery.title, delivery.body
	result.ThreadID = "raven-notices"
	result.Sound = &notifications.NotificationSound{Silent: true, Name: ""}

	return result
}
