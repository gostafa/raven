// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"raven/internal/autostart"
)

func (b backend) EnableWithOptions(v autostart.Options) error {
	return b.native.EnableWithOptions(
		application.AutostartOptions{Identifier: v.Identifier, Arguments: v.Arguments},
	)
}

func (b backend) Disable() error { return b.native.Disable() }

func (b backend) IsEnabled() (bool, error) { return b.native.IsEnabled() }

func New(native *application.AutostartManager) autostart.Backend { return backend{native: native} }
