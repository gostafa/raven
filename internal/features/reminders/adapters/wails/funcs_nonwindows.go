//go:build !windows

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

func recordNotificationStartupCleanupFailure(
	_ *notificationLifecycle,
	_ error,
) {
}
