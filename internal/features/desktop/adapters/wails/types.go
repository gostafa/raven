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
	reminderwails "raven/internal/features/reminders/adapters/wails"
	reminderports "raven/internal/features/reminders/ports"
	mediaports "raven/internal/shared/desktopmedia/ports"
)

type (
	// Configuration supplies shared geometry, identity, events, and logging names.
	Configuration struct {
		ApplicationID string
		LogErrorKey   string
		ShowReviews   string
		Layout        companion.Layout
	}

	// ApplicationDependencies supplies the shell without depending on native windows.
	ApplicationDependencies struct {
		Configuration *Configuration
		Media         mediaports.MediaRoutes
		Visibility    *autostart.Visibility
		Logger        *slog.Logger
		Assets        ports.Assets
	}
	// Dependencies supplies stable runtime contracts to desktop orchestration.
	Dependencies struct {
		Configuration *Configuration
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
		windows       companion.Windows
		reminders     reminderports.Control
		store         autostart.Settings
		mainWindow    *application.WebviewWindow
		widget        *application.WebviewWindow
		reviewWindow  *application.WebviewWindow
		app           *application.App
		visibility    *autostart.Visibility
		startup       *autostart.Controller
		notifications *reminderwails.Runtime
		subscriptions []func()
		icon          []byte
		configuration Configuration
	}
	// EventPublisher publishes Wails events through a consumer-owned contract.
	EventPublisher struct{ app *application.App }
)
