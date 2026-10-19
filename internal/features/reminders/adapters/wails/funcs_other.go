//go:build !darwin

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

func reminderSound() *notifications.NotificationSound { return nil }

func playReminder(context.Context, func() (string, error)) error { return nil }
