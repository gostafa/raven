// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package application defines login startup policy and launch visibility.
package application

import (
	"sync"
)

type (

	// Controller serializes login registration and publishes the observed OS state.
	Controller struct {
		execute func(string) error
		publish func(func(bool, bool))
	}

	registration struct {
		claim   func() (bool, error)
		status  func() (bool, error)
		enable  func() error
		disable func() error
		publish func(bool, bool)
		mu      sync.Mutex
		debug   bool
		ready   bool
		enabled bool
	}

	// Visibility reconciles login classification, readiness, and coalesced opening requests.
	Visibility struct {
		execute func(string, bool) <-chan struct{}
	}

	launchState struct {
		show       func()
		pending    chan struct{}
		mu         sync.Mutex
		started    bool
		classified bool
		automatic  bool
		open       bool
		settled    bool
	}

	// LaunchClassification receives the detected launch kind independently of native callbacks.
	LaunchClassification interface{ Classify(automatic bool) }

	// Configuration supplies the identity and development mode for login registration.
	Configuration struct {
		// ApplicationID identifies the login registration.
		ApplicationID string
		// Debug disables registration changes during development.
		Debug bool
	}
)
