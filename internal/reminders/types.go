// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package reminders defines review reminder reconciliation and delivery.
package reminders

import (
	"context"
	"sync"

	"raven/internal/library"
)

type (
	// Store acknowledges notification delivery for unchanged item schedules.
	Store interface {
		MarkNotified(occurrences []library.DueOccurrence) error
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
		ListCards() ([]library.Card, error)
		ClaimNotificationPrompt() (bool, error)
	}

	// Coordinator reconciles review reminders until stopped.
	Coordinator struct {
		cancel   context.CancelFunc
		changed  chan struct{}
		done     chan struct{}
		stopOnce sync.Once
	}
)
