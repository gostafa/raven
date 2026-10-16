// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package wails adapts Wails login startup registration.
package wails

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

type (
	// Backend translates login startup registration to Wails.
	Backend struct{ native NativeAutostart }

	// NativeAutostart supplies the native operations used by the adapter.
	NativeAutostart interface {
		EnableWithOptions(options application.AutostartOptions) error
		Disable() error
		IsEnabled() (bool, error)
	}
)
