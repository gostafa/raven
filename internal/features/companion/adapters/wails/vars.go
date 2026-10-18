// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	companion "raven/internal/features/companion/domain"
	"raven/internal/features/companion/ports"
)

var (
	_ mainWindow        = (*application.WebviewWindow)(nil)
	_ widgetSurface     = (*application.WebviewWindow)(nil)
	_ reviewSurface     = (*application.WebviewWindow)(nil)
	_ reviewFrame       = (*application.WebviewWindow)(nil)
	_ companion.Windows = (*Windows)(nil)

	_ ports.WidgetConfiguration = (*Widget)(nil)
)
