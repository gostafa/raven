// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"raven/internal/features/companion/adapters/native"
	"raven/internal/features/companion/adapters/wails"
	companion "raven/internal/features/companion/domain"
	"raven/internal/features/companion/ports"
)

func newConfiguration(handles wails.WindowSet) ports.WidgetConfiguration {
	return native.New(handles.Widget)
}

func newWindows(params windowParams) companion.Windows {
	return wails.New(params.App, params.Handles, params.Configuration)
}
