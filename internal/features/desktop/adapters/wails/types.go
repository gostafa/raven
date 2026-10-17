// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package wails defines wails contracts for adapters.
package wails

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	autostart "raven/internal/features/autostart/application"
	companionwails "raven/internal/features/companion/adapters/wails"
	companion "raven/internal/features/companion/domain"
	"raven/internal/features/desktop/ports"
	libraryports "raven/internal/features/library/ports"
	reminderwails "raven/internal/features/reminders/adapters/wails"
	reminderports "raven/internal/features/reminders/ports"
)

type (
	// ApplicationDependencies supplies the shell without depending on native windows.
	ApplicationDependencies struct {
		Media      libraryports.MediaRoutes
		Visibility *autostart.Visibility
		Logger     *slog.Logger
		Assets     ports.Assets
	}
	// Dependencies supplies stable runtime contracts to desktop orchestration.
	Dependencies struct {
		App           *application.App
		Handles       companionwails.WindowSet
		Windows       companion.Windows
		Settings      autostart.Settings
		Visibility    *autostart.Visibility
		Startup       *autostart.Controller
		Reminders     reminderports.Control
		Notifications *reminderwails.Runtime
		Assets        ports.Assets
	}
	// Desktop owns native desktop orchestration for the lifetime of Wails.
	Desktop struct {
		subscriptions []func()
		app           *application.App
		mainWindow    *application.WebviewWindow
		widget        *application.WebviewWindow
		reviewWindow  *application.WebviewWindow
		windows       companion.Windows
		store         autostart.Settings
		visibility    *autostart.Visibility
		startup       *autostart.Controller
		reminders     reminderports.Control
		notifications *reminderwails.Runtime
		icon          []byte
	}
	// EventPublisher publishes Wails events through a consumer-owned contract.
	EventPublisher struct{ app *application.App }
)
