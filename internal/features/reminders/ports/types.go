// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines ports contracts for reminders.
package ports

type (
	// Control starts reconciliation on desktop readiness and wakes or stops its worker.
	Control interface {
		Start()
		Wake()
		Stop()
	}
	// Store acknowledges notification delivery for unchanged item schedules.
	Store interface {
		MarkNotified(occurrences string) error
	}
	// Notifier provides notification authorization and reminder delivery.
	Notifier interface {
		CheckNotificationAuthorization() (bool, error)
		Send(count int) error
		RequestAuthorization() (bool, error)
	}
)
