// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package application defines login startup policy and launch visibility.
package application

import (
	"sync"
)

type (
	// Backend provides operating system login startup registration.
	Backend interface {
		EnableWithOptions(options *Options) error
		Disable() error
		IsEnabled() (bool, error)
	}

	// Settings claims the one-time login startup initialization attempt.
	Settings interface {
		ClaimAutostartInitialization() (bool, error)
	}

	// Controller serializes login startup changes and publishes the observed OS state.
	Controller struct {
		backend       Backend
		publish       func(checked, available bool)
		applicationID string
		mu            sync.Mutex
		debug         bool
		ready         bool
		enabled       bool
	}

	// Visibility reconciles launch classification with explicit window opening requests.
	Visibility struct {
		show       func()
		mu         sync.Mutex
		started    bool
		classified bool
		automatic  bool
		open       bool
		settled    bool
	}

	// Options describes the application identity and arguments registered at login.
	Options struct {
		// Identifier is the operating system application identifier.
		Identifier string
		// Arguments contains the arguments supplied on an automatic launch.
		Arguments []string
	}
)
