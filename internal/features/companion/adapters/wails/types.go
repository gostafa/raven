// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package wails adapts Raven window operations to Wails.
package wails

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	companion "raven/internal/features/companion/domain"
	companionports "raven/internal/features/companion/ports"
)

type (
	// Dependencies supplies native windows, styling, and immutable window settings.
	Dependencies struct {
		App           *application.App
		Handles       WindowSet
		Configuration companionports.WidgetConfiguration
		Settings      Settings
	}

	// Settings supplies geometry and shared event/log names to window orchestration.
	Settings struct {
		LogErrorKey         string
		OpenFloatingReview  string
		CloseFloatingReview string
		Layout              companion.Layout
	}

	// Windows owns the Wails windows used by Raven.
	Windows struct {
		mainWindow      restorableWindow
		widget          widgetSurface
		reviewWindow    reviewSurface
		reviewPlacement windowFrame
		configuration   companionports.WidgetConfiguration
		invoke          func(func())
		primaryScreen   func() *application.Screen
		quit            func()
		logger          *slog.Logger
		settings        Settings
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
