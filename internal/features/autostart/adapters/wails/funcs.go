// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"errors"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// EnableWithOptions registers the supplied application identity and arguments.
func (backend Backend) EnableWithOptions(identifier string, arguments []string) error {
	return errors.Join(
		registrationFailure(
			backend.execute(
				enableCommand,
				&application.AutostartOptions{Identifier: identifier, Arguments: arguments},
			),
		),
	)
}

// Disable removes login startup registration.
func (backend Backend) Disable() error {
	return errors.Join(registrationFailure(backend.execute(disableCommand, nil)))
}

// IsEnabled reports the observed operating system registration.
func (backend Backend) IsEnabled() (bool, error) {
	enabled, err := backend.execute("status", nil)

	return enabled, errors.Join(registrationError("is enabled", err))
}

func registrationError(operation string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return nil
}

// New binds the consumer-owned registration port to its native implementation.
func New(native NativeAutostart) Backend {
	return Backend{
		execute: func(operation string, options *application.AutostartOptions) (bool, error) {
			return nativeRegistration(native, operation, options)
		},
	}
}

func nativeRegistration(
	native NativeAutostart,
	operation string,
	options *application.AutostartOptions,
) (bool, error) {
	switch operation {
	case enableCommand:
		return false, errors.Join(native.EnableWithOptions(*options))
	case disableCommand:
		return false, errors.Join(native.Disable())
	default:
		enabled, err := native.IsEnabled()

		return enabled, errors.Join(err)
	}
}

// NewDetector constructs native login launch detection.
func NewDetector() Detector { return Detector{} }

func registrationFailure(_ bool, err error) error { return errors.Join(err) }
