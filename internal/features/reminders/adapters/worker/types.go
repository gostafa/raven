// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package worker

import (
	"sync"

	reminders "raven/internal/features/reminders/application"
	"raven/internal/features/reminders/ports"
)

type (
	// Worker owns one reminder coordinator after both lifecycle and UI readiness.
	Worker struct {
		source        reminders.Source
		notifications ports.Notifications
		callbacks     reminders.Callbacks
		coordinator   *reminders.Coordinator
		mu            sync.Mutex
		enabled       bool
		stopped       bool
	}
)
