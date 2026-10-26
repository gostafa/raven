// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package notifications adapts application messages to native OS notifications.
package notifications

import (
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type (
	// Native supplies the native delivery operations used by the adapter.
	Native interface {
		CheckNotificationAuthorization() (bool, error)
		SendNotification(options notifications.NotificationOptions) error
	}
	// Notifier delivers application messages through an existing native runtime.
	Notifier struct{ send func(string, string) error }
	message  = struct{ title, body string }
)
