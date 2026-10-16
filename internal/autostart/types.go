// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package autostart defines login startup policy and launch visibility.
package autostart

import (
	"sync"
)

type Backend interface {
	EnableWithOptions(Options) error
	Disable() error
	IsEnabled() (bool, error)
}

type Settings interface {
	ClaimAutostartInitialization() (bool, error)
}

// All OS operations and menu publications share this lock, including refreshes.
// Read OS state before toggling: a checkbox may be stale after external changes.
type Controller struct {
	backend Backend
	publish func(checked, available bool)
	mu      sync.Mutex
	debug   bool
	ready   bool
	enabled bool
}

// Native launch classification and ApplicationStarted may arrive in either order.
// Explicit open requests arriving during startup are retained until both are ready.
type Visibility struct {
	show       func()
	mu         sync.Mutex
	started    bool
	classified bool
	automatic  bool
	open       bool
	settled    bool
}

type Options struct {
	Identifier string
	Arguments  []string
}
