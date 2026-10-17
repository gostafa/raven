// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	reminders "raven/internal/features/reminders/application"
	reminderports "raven/internal/features/reminders/ports"
)

var (
	_ NativeRuntime           = (*notifications.NotificationService)(nil)
	_ reminders.Notifications = (*Runtime)(nil)
	_ reminderports.Notifier  = (*Notifier)(nil)
)
