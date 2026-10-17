// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package application defines review reminder reconciliation and delivery.
package application

import (
	"sync"
	"time"

	cardapi "raven/api/cards"
	reviewapi "raven/api/reviews"
	"raven/internal/features/reminders/ports"
)

type (

	// Source provides persisted schedules and notification prompt state.
	Source interface {
		ports.Store
		ListCards() ([]cardapi.Card, error)
		ClaimNotificationPrompt() (bool, error)
	}

	// Coordinator reconciles review reminders until stopped.
	Coordinator struct {
		source    Source
		notifier  ports.Notifier
		callbacks Callbacks
		stopping  chan struct{}
		changed   chan struct{}
		done      chan struct{}
		stopOnce  sync.Once
	}

	// Callbacks publishes schedule changes and reports reminder failures.
	Callbacks struct {
		// Publish announces that the current schedule snapshot is available.
		Publish func()
		// Warn reports an operation and its failure without stopping reconciliation.
		Warn func(string, error)
	}

	reminderSnapshot struct {
		now     time.Time
		pending []reviewapi.DueOccurrence
		delay   time.Duration
	}

	// Worker owns one reminder coordinator after both lifecycle and UI readiness.
	Worker struct {
		source        Source
		notifications Notifications
		callbacks     Callbacks
		coordinator   *Coordinator
		mu            sync.Mutex
		enabled       bool
		stopped       bool
	}

	// Notifications combines delivery with the observed native availability.
	Notifications interface {
		ports.Notifier
		Available() bool
	}
)
