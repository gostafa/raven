// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package reminders defines review reminder reconciliation and delivery.
package reminders

import (
	"context"
	"sync"

	"raven/internal/library"
)

type Store interface {
	MarkNotified([]library.DueOccurrence) error
}

type Notifier interface {
	CheckNotificationAuthorization() (bool, error)
	Send(int) error
	RequestAuthorization() error
}

type Source interface {
	Store
	ListCards() ([]library.Card, error)
	ClaimNotificationPrompt() (bool, error)
}
type Coordinator struct {
	cancel   context.CancelFunc
	changed  chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}
