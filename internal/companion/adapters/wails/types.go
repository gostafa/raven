// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package wails adapts Raven window operations to Wails.
package wails

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

type (
	// Windows owns the Wails windows used by Raven.
	Windows struct {
		app                              *application.App
		mainWindow, widget, reviewWindow *application.WebviewWindow
	}
	// WindowSet contains the Wails windows controlled by this adapter.
	WindowSet struct {
		// Main is the main library window.
		Main *application.WebviewWindow
		// Widget is the floating launcher window.
		Widget *application.WebviewWindow
		// Review is the floating review window.
		Review *application.WebviewWindow
	}
)
