// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package application defines review reminder reconciliation and delivery.
package application

import (
	"sync"
	"time"

	cardapi "raven/api/cards"
	reviewapi "raven/api/reviews"
)

type (
	// Store acknowledges notification delivery for unchanged item schedules.
	Store interface {
		MarkNotified(occurrences []reviewapi.DueOccurrence) error
	}

	// Notifier provides notification authorization and reminder delivery.
	Notifier interface {
		CheckNotificationAuthorization() (bool, error)
		Send(count int) error
		RequestAuthorization() error
	}

	// Source provides persisted schedules and notification prompt state.
	Source interface {
		Store
		ListCards() ([]cardapi.Card, error)
		ClaimNotificationPrompt() (bool, error)
	}

	// Coordinator reconciles review reminders until stopped.
	Coordinator struct {
		source    Source
		notifier  Notifier
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
)
