// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package wails defines wails contracts for adapters.
package wails

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"raven/internal/features/desktop/ports"
)

type (
	windowEventSource interface {
		OnWindowEvent(kind events.WindowEventType, callback func(*application.WindowEvent)) func()
	}
	applicationEventSource interface {
		OnApplicationEvent(
			kind events.ApplicationEventType,
			callback func(*application.ApplicationEvent),
		) func()
	}
	// Configuration supplies native geometry and application identity.
	Configuration struct {
		// ApplicationID identifies the single native application instance.
		ApplicationID string
		// WidgetSize is the floating launcher diameter.
		WidgetSize int
		// ReviewWidth is the preferred review width.
		ReviewWidth int
		// ReviewHeight is the preferred review height.
		ReviewHeight int
	}
	// ApplicationDependencies supplies the shell before native windows exist.
	ApplicationDependencies = struct {
		// ApplicationID identifies the single native application instance.
		ApplicationID string
		// Middleware serves application-owned media.
		Middleware func(http.Handler) http.HandlerFunc
		// SecondInstance handles subsequent launch arguments.
		SecondInstance func([]string)
		// Logger receives structured native logs.
		Logger *slog.Logger
		// Assets contains the embedded frontend.
		Assets fs.FS
		// Icon contains the native application icon.
		Icon []byte
	}
	// Dependencies supplies native handles and consumer-owned operation capabilities.
	Dependencies = struct {
		Localization               ports.Localization
		InitializeStartup          func() error
		OnReview                   func(func())
		Widget                     *application.WebviewWindow
		Review                     *application.WebviewWindow
		ShowMain                   func()
		HideMain                   func()
		ConfigureWidget            func()
		ResetWidget                func()
		PositionFloatingReview     func()
		RequestFloatingReviewClose func()
		Quit                       func()
		ApplicationStarted         func()
		RequestOpen                func()
		App                        *application.App
		Main                       *application.WebviewWindow
		SetStartupPublisher        func(func(bool, bool))
		RefreshStartup             func() error
		StartReminders             func()
		WakeReminders              func()
		ToggleStartup              func() error
		showDialog                 func(*application.MessageDialog)
		run                        func() error
		invoke                     func(func())
		post                       func(func())
		LogErrorKey                string
		ShowReviews                string
		Icon                       []byte
		subscriptions              []func()
	}
	// Desktop dispatches registered native lifecycle operations.
	Desktop struct {
		operations map[string]func(context.Context, *application.Service, <-chan struct{}) desktopResult
	}
	desktopResult struct{ failure error }
	// EventPublisher publishes Wails events through a consumer-owned contract.
	EventPublisher struct{ app *application.App }
)
