// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package retry bounds transient operations and owns shared initialization.
package retry

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gostafa/singleton"
)

type (
	// Operation adapts a context-aware retry callback to the media reader port.
	Operation func(context.Context, func(context.Context) error) error
	event     = struct {
		err     error
		attempt int
		delay   time.Duration
	}
	// Options supplies resource ownership, classification, and diagnostics.
	Options[Value any] = struct {
		Logger    *slog.Logger
		Release   func(Value) error
		Transient func(error) bool
		Name      string
	}
	// Reads retries safe loads under their caller and application lifetimes.
	Reads struct {
		owner  func() context.Context
		logger *slog.Logger
	}
	// Initialization owns one resource and drains its factory before closing it.
	Initialization[Value any] struct {
		get  func(context.Context) (Value, error)
		stop func() error
	}
	initializationState[T any] = struct {
		value     T
		err       error
		closeErr  error
		provider  *singleton.Provider[T]
		cancel    context.CancelFunc
		release   func(T) error
		done      chan struct{}
		closeOnce sync.Once
		mu        sync.Mutex
		started   bool
		stopped   bool
	}
	policy = struct {
		logger    *slog.Logger
		transient func(error) bool
		name      string
		level     slog.Level
		attempts  uint
		timeout   time.Duration
		initial   time.Duration
		maximum   time.Duration
	}
)
