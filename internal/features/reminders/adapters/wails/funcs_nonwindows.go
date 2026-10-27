//go:build !windows

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"errors"
	"net"

	resilience "raven/internal/shared/resilience/adapters/retry"
)

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
