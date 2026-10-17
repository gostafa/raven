// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines native login launch detection.
package ports

type (
	// LoginDetection registers the callback that classifies an operating system login launch.
	LoginDetection interface {
		PrepareLoginDetection(detected func(bool))
	}
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
	// Options describes the application identity and arguments registered at login.
	Options struct {
		// Identifier is the operating system application identifier.
		Identifier string
		// Arguments contains the arguments supplied on an automatic launch.
		Arguments []string
	}
)
