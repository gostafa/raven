//go:build windows

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import "errors"

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
