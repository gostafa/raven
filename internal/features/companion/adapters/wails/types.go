// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package wails adapts Raven window operations to Wails.
package wails

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type (
	mainWindow interface {
		Show() application.Window
		Hide() application.Window
		Restore()
		Focus()
	}
	mainFullscreenWindow interface {
		IsFullscreen() bool
		UnFullscreen()
		OnWindowEvent(kind events.WindowEventType, callback func(*application.WindowEvent)) func()
	}
	widgetSurface interface {
		Show() application.Window
		Hide() application.Window
		Position() (int, int)
		SetPosition(horizontal, vertical int)
		GetScreen() (*application.Screen, error)
	}
	reviewSurface interface {
		Show() application.Window
		Hide() application.Window
		Focus()
		IsVisible() bool
		DispatchWailsEvent(event *application.CustomEvent)
	}
	reviewFrame interface {
		SetSize(width, height int) application.Window
		SetPosition(horizontal, vertical int)
	}
	// Dependencies supplies native handles and widget styling.
	Dependencies struct {
		// App owns the native application lifetime.
		App *application.App
		// Main is the main library window.
		Main *application.WebviewWindow
		// Widget is the floating launcher.
		Widget *application.WebviewWindow
		// Review is the floating review window, when present.
		Review *application.WebviewWindow
		// Configure applies platform widget styling.
		Configure func() error
	}
	// Settings supplies native geometry and event names.
	Settings struct {
		// LogErrorKey names the structured error attribute.
		LogErrorKey string
		// OpenFloatingReview names the review opening event.
		OpenFloatingReview string
		// CloseFloatingReview names the review closing event.
		CloseFloatingReview string
		// WidgetSize is the launcher width and height.
		WidgetSize int
		// ReviewWidth is the preferred review width.
		ReviewWidth int
		// ReviewHeight is the preferred review height.
		ReviewHeight int
		// ReviewGap separates the launcher and review.
		ReviewGap int
	}
	// Windows dispatches the consumer-owned native window operations.
	Windows     struct{ operations map[string]func() }
	windowState struct {
		logger              *slog.Logger
		setReviewPosition   func(int, int)
		restoreMain         func()
		focusMain           func()
		mainFullscreen      func() bool
		exitMainFullscreen  func()
		setReviewSize       func(int, int) application.Window
		showWidget          func() application.Window
		hideWidget          func() application.Window
		reviewVisible       func() bool
		setWidgetPosition   func(int, int)
		getWidgetScreen     func() (*application.Screen, error)
		showReview          func() application.Window
		hideReview          func() application.Window
		focusReview         func()
		primaryScreen       func() *application.Screen
		hideMain            func() application.Window
		widgetPosition      func() (int, int)
		showMain            func() application.Window
		configure           func() error
		invoke              func(func())
		dispatchReview      func(*application.CustomEvent)
		quit                func()
		observeMainExit     func(func(*application.WindowEvent)) func()
		openFloatingReview  string
		closeFloatingReview string
		logErrorKey         string
		widgetSize          int
		reviewWidth         int
		reviewHeight        int
		reviewGap           int
		hideMainPending     bool
		mainExitObserved    bool
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
