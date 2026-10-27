//go:build windows

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

func reminderSound() *notifications.NotificationSound { return nil }

func playReminder(context.Context, func() (string, error)) error { return nil }

func recordNotificationStartupCleanupFailure(
	state *notificationLifecycle,
	err error,
) {
	if !errors.Is(err, errNotificationCleanup) {
		return
	}

	state.mu.Lock()
	state.startupCleanupError = err
	state.mu.Unlock()
}
