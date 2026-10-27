//go:build !darwin && !windows

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"context"
	"errors"
	"net"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	resilience "raven/internal/shared/resilience/adapters/retry"
)

func reminderSound() *notifications.NotificationSound { return nil }

func notificationTransient(err error) bool {
	if errors.Is(err, errNotificationCleanup) {
		return false
	}

	if resilience.Transient(err) {
		return true
	}

	var failure *net.OpError

	return errors.As(err, &failure) && failure.Timeout()
}

func playReminder(context.Context, func() (string, error)) error { return nil }

func recordNotificationStartupCleanupFailure(*notificationLifecycle, error) {}
