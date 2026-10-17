// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines ports contracts for reminders.
package ports

import (
	reminders "raven/internal/features/reminders/application"
)

type (
	// Control starts reconciliation on desktop readiness and wakes or stops its worker.
	Control interface {
		Start()
		Wake()
		Stop()
	}

	// Notifications combines delivery with the observed native availability.
	Notifications interface {
		reminders.Notifier
		Available() bool
	}
)
