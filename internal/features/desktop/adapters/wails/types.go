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
	ApplicationDependencies struct {
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
	Dependencies struct {
		// App owns the native application.
		App *application.App
		// Main is the main library window.
		Main *application.WebviewWindow
		// Widget is the launcher window.
		Widget *application.WebviewWindow
		// Review is the floating review window.
		Review *application.WebviewWindow
		// ShowMain reveals the main library.
		ShowMain func()
		// HideMain hides the main library.
		HideMain func()
		// ConfigureWidget applies native launcher styling.
		ConfigureWidget func()
		// ResetWidget restores launcher placement.
		ResetWidget func()
		// PositionFloatingReview places the review beside the launcher.
		PositionFloatingReview func()
		// RequestFloatingReviewClose asks the frontend to finish and close.
		RequestFloatingReviewClose func()
		// Quit terminates the application.
		Quit func()
		// ApplicationStarted releases launch visibility after native readiness.
		ApplicationStarted func()
		// RequestOpen requests visible launch behavior.
		RequestOpen func()
		// InitializeStartup reconciles login registration after readiness.
		InitializeStartup func() error
		// RefreshStartup reads the current login registration state.
		RefreshStartup func() error
		// ToggleStartup changes login registration.
		ToggleStartup func() error
		// SetStartupPublisher observes checkbox state and availability.
		SetStartupPublisher func(func(bool, bool))
		// StartReminders releases reminders after native readiness.
		StartReminders func()
		// WakeReminders reconciles reminders after wake.
		WakeReminders func()
		// OnReview observes successful default notification actions.
		OnReview func(func())
		// ShowReviews names the frontend review event.
		ShowReviews string
		// LogErrorKey names the structured error attribute.
		LogErrorKey string
		// Icon contains the native application icon.
		Icon          []byte
		run           func() error
		invoke        func(func())
		showDialog    func(*application.MessageDialog)
		subscriptions []func()
	}
	// Desktop dispatches registered native lifecycle operations.
	Desktop struct {
		operations map[string]func(context.Context, *application.Service, <-chan struct{}) desktopResult
	}
	desktopResult struct{ failure error }
	// EventPublisher publishes Wails events through a consumer-owned contract.
	EventPublisher struct{ app *application.App }
)
