// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

func reminderSound() *notifications.NotificationSound {
	return &notifications.NotificationSound{Name: "raven.wav", Silent: false}
}
