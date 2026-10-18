// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package application defines review reminder reconciliation and delivery.
package application

import (
	"sync"
	"time"

	"raven/internal/features/reminders/ports"
)

type (

	// Source provides JSON card records and durable notification prompt state.
	Source interface {
		ports.Store
		ListCards() (string, error)
		ClaimNotificationPrompt() (bool, error)
	}

	// Coordinator exposes cancellation and coalesced reconciliation requests.
	Coordinator struct {
		wake func()
		stop func()
	}

	coordinatorSignals struct {
		stopping chan struct{}
		changed  chan struct{}
		done     chan struct{}
		stopOnce sync.Once
	}

	reconciliation struct {
		list      func() (string, error)
		claim     func() (bool, error)
		authorize func() (bool, error)
		deliver   func([]byte) error
		publish   func()
		warn      func(string, error)
		wake      func()
		wait      func(time.Duration) bool
		finish    func()
	}

	// Callbacks publishes schedule changes and reports reminder failures.
	Callbacks struct {
		// Publish announces that the current schedule snapshot is available.
		Publish func()
		// Warn reports an operation and its failure without stopping reconciliation.
		Warn func(string, error)
	}

	// Worker owns reminder reconciliation after lifecycle and UI readiness.
	Worker struct{ execute func(string) }

	workerState struct {
		start   func() (func(), func())
		wake    func()
		stop    func()
		mu      sync.Mutex
		enabled bool
		stopped bool
	}

	// Notifications combines delivery with the observed native availability.
	Notifications interface {
		ports.Notifier
		Available() bool
	}
)
