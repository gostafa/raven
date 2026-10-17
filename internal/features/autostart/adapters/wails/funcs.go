// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	autostartports "raven/internal/features/autostart/ports"
)

// EnableWithOptions enables login startup with the supplied application options.
func (backend Backend) EnableWithOptions(visibility *autostartports.Options) error {
	forwardErr3 := backend.native.EnableWithOptions(
		application.AutostartOptions{
			Identifier: visibility.Identifier,
			Arguments:  visibility.Arguments,
		},
	)
	if forwardErr3 != nil {
		return fmt.Errorf("enable with options: %w", forwardErr3)
	}

	return nil
}

// Disable disables login startup registration.
func (backend Backend) Disable() error {
	forwardErr := backend.native.Disable()
	if forwardErr != nil {
		return fmt.Errorf("disable: %w", forwardErr)
	}

	return nil
}

// IsEnabled returns the current operating system login startup state.
func (backend Backend) IsEnabled() (bool, error) {
	result, forwardErr2 := backend.native.IsEnabled()
	if forwardErr2 != nil {
		return result, fmt.Errorf("is enabled: %w", forwardErr2)
	}

	return result, nil
}

// New constructs the adapter with its required runtime dependencies.
func New(native NativeAutostart) Backend { return Backend{native: native} }

// NewDetector constructs native login launch detection.
func NewDetector() Detector { return Detector{} }
