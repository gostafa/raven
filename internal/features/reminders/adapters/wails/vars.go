// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"raven/internal/features/reminders"
)

var _ reminders.Notifier = (*Notifier)(nil)
