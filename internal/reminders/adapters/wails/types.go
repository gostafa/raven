// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package wails adapts reminder delivery to native Wails notifications.
package wails

import (
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type (
	// Notifier translates reminder delivery to native notifications.
	Notifier struct {
		native NativeNotifications
	}

	// NativeNotifications supplies the native operations used by the adapter.
	NativeNotifications interface {
		CheckNotificationAuthorization() (bool, error)
		SendNotification(options notifications.NotificationOptions) error
		RequestNotificationAuthorization() (bool, error)
	}
)
