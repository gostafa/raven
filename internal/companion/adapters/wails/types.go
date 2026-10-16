// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

type windows struct {
	app                              *application.App
	mainWindow, widget, reviewWindow *application.WebviewWindow
}
