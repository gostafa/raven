// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package wails adapts Raven window operations to Wails.
package wails

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type (
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
		showMain            func() application.Window
		hideMain            func() application.Window
		restoreMain         func()
		focusMain           func()
		showWidget          func() application.Window
		widgetPosition      func() (int, int)
		setWidgetPosition   func(int, int)
		getWidgetScreen     func() (*application.Screen, error)
		showReview          func() application.Window
		hideReview          func() application.Window
		focusReview         func()
		reviewVisible       func() bool
		dispatchReview      func(*application.CustomEvent)
		setReviewSize       func(int, int) application.Window
		setReviewPosition   func(int, int)
		configure           func() error
		invoke              func(func())
		primaryScreen       func() *application.Screen
		quit                func()
		logger              *slog.Logger
		logErrorKey         string
		openFloatingReview  string
		closeFloatingReview string
		widgetSize          int
		reviewWidth         int
		reviewHeight        int
		reviewGap           int
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
