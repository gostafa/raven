// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	reminders "raven/internal/features/reminders/application"
)

var _ reminders.Notifier = (*Notifier)(nil)
