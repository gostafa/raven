// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"raven/internal/features/companion/adapters/native"
	"raven/internal/features/companion/adapters/wails"
)

func newConfiguration(handles wails.WindowSet) native.Widget {
	return native.New(handles.Widget)
}

func newWindows(params windowParams) *wails.Windows {
	return wails.New(params.App, params.Handles, params.Configuration)
}
