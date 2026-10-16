// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package wails adapts Raven window operations to Wails.
package wails

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type (
	// Windows owns the Wails windows used by Raven.
	Windows struct {
		mainWindow      restorableWindow
		widget          widgetSurface
		reviewWindow    reviewSurface
		reviewPlacement windowFrame
		nativeWidget    *application.WebviewWindow
		configureNative func(*application.WebviewWindow) error
		invoke          func(func())
		primaryScreen   func() *application.Screen
		quit            func()
		logger          *slog.Logger
	}
	windowVisibility interface {
		Show() application.Window
		Hide() application.Window
	}
	windowFocus  interface{ Focus() }
	windowEvents interface {
		DispatchWailsEvent(event *application.CustomEvent)
	}
	windowPosition interface {
		Position() (int, int)
		SetPosition(horizontal, vertical int)
	}
	restorableWindow interface {
		windowVisibility
		windowFocus
		Restore()
	}
	widgetSurface interface {
		windowPosition
		Show() application.Window
		GetScreen() (*application.Screen, error)
	}
	reviewSurface interface {
		windowVisibility
		windowFocus
		windowEvents
		IsVisible() bool
	}
	windowFrame interface {
		SetSize(width, height int) application.Window
		SetPosition(horizontal, vertical int)
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
