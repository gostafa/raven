// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	autostartports "raven/internal/features/autostart/ports"
)

// EnableWithOptions registers the supplied application identity and arguments.
func (backend Backend) EnableWithOptions(options *autostartports.Options) error {
	_, err := backend.execute(
		"enable",
		application.AutostartOptions{Identifier: options.Identifier, Arguments: options.Arguments},
	)
	return registrationError("enable with options", err)
}

// Disable removes login startup registration.
func (backend Backend) Disable() error {
	_, err := backend.execute("disable", application.AutostartOptions{})
	return registrationError("disable", err)
}

// IsEnabled reports the observed operating system registration.
func (backend Backend) IsEnabled() (bool, error) {
	enabled, err := backend.execute("status", application.AutostartOptions{})
	return enabled, registrationError("is enabled", err)
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
		execute: func(operation string, options application.AutostartOptions) (bool, error) {
			return nativeRegistration(native, operation, options)
		},
	}
}

func nativeRegistration(
	native NativeAutostart,
	operation string,
	options application.AutostartOptions,
) (bool, error) {
	switch operation {
	case "enable":
		return false, native.EnableWithOptions(options)
	case "disable":
		return false, native.Disable()
	default:
		return native.IsEnabled()
	}
}

// NewDetector constructs native login launch detection.
func NewDetector() Detector { return Detector{} }
