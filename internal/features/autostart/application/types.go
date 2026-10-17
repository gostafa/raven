// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package application defines login startup policy and launch visibility.
package application

import (
	"sync"

	"raven/internal/features/autostart/ports"
)

type (

	// Controller serializes login startup changes and publishes the observed OS state.
	Controller struct {
		backend       ports.Backend
		settings      ports.Settings
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

	// Requests decouples launch classification from native window construction.
	Requests struct {
		visibility *Visibility
		pending    chan struct{}
	}

	// LaunchClassification receives the detected launch kind independently of native callbacks.
	LaunchClassification interface{ Classify(automatic bool) }

	// Configuration supplies the identity and development mode for login registration.
	Configuration struct {
		ApplicationID string
		Debug         bool
	}
)
