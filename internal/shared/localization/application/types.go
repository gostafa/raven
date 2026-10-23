// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package application coordinates durable language changes across Raven windows.
package application

import (
	"sync"

	"raven/internal/shared/localization/domain"
	"raven/internal/shared/localization/ports"
)

type (
	// Service owns the application language capability.
	Service struct {
		operations map[string]func(string, func()) result
	}
	result = struct {
		failure error
		cancel  func()
		state   domain.State
	}
	state = struct {
		preferences ports.Preferences
		system      ports.SystemLocales
		warn        func(error)
		listeners   map[uint64]func()
		current     domain.State
		next        uint64
		mu          sync.RWMutex
		change      sync.Mutex
	}
)
