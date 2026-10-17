// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package worker

import (
	reminders "raven/internal/features/reminders/application"
	"raven/internal/features/reminders/ports"
)

// New constructs reconciliation without starting background work.
func New(
	source reminders.Source,
	notifications ports.Notifications,
	callbacks reminders.Callbacks,
) *Worker {
	var worker Worker

	worker.source = source
	worker.notifications = notifications
	worker.callbacks = callbacks

	return &worker
}

// Enable permits the readiness event to start reconciliation.
func (worker *Worker) Enable() {
	worker.mu.Lock()
	defer worker.mu.Unlock()

	worker.enabled = true
}

// Start begins exactly one coordinator when the native application is ready.
func (worker *Worker) Start() {
	worker.mu.Lock()
	defer worker.mu.Unlock()

	if !worker.enabled || worker.stopped || worker.coordinator != nil {
		return
	}

	var notifier reminders.Notifier

	if worker.notifications.Available() {
		notifier = worker.notifications
	}

	worker.coordinator = reminders.Start(worker.source, notifier, worker.callbacks)
}

// Wake coalesces reconciliation requests before and after readiness.
func (worker *Worker) Wake() {
	worker.mu.Lock()
	defer worker.mu.Unlock()

	if worker.coordinator != nil && !worker.stopped {
		worker.coordinator.Wake()
	}
}

// Stop prevents late readiness events and waits for active reconciliation.
func (worker *Worker) Stop() {
	worker.mu.Lock()

	worker.stopped = true

	coordinator := worker.coordinator
	worker.mu.Unlock()

	if coordinator != nil {
		coordinator.Stop()
	}
}
