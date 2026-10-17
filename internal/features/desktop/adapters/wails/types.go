// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package wails defines wails contracts for adapters.
package wails

import (
	"io/fs"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

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
		Visibility    ports.LaunchVisibility
		Logger        *slog.Logger
		Assets        fs.FS
		Icon          []byte
	}
	// Dependencies supplies stable runtime contracts to desktop orchestration.
	Dependencies struct {
		Configuration *Configuration
		App           *application.App
		Handles       companionwails.WindowSet
		Windows       companion.Windows
		Visibility    ports.LaunchVisibility
		Startup       ports.Startup
		Reminders     reminderports.Control
		Notifications *reminderwails.Runtime
		Assets        fs.FS
		Icon          []byte
	}
	// Desktop owns native desktop orchestration for the lifetime of Wails.
	Desktop struct {
		run           func() error
		invoke        func(func())
		showDialog    func(*application.MessageDialog)
		windows       companion.Windows
		reminders     reminderports.Control
		mainWindow    *application.WebviewWindow
		widget        *application.WebviewWindow
		reviewWindow  *application.WebviewWindow
		app           *application.App
		visibility    ports.LaunchVisibility
		startup       ports.Startup
		notifications *reminderwails.Runtime
		subscriptions []func()
		icon          []byte
		configuration Configuration
	}
	// EventPublisher publishes Wails events through a consumer-owned contract.
	EventPublisher struct{ app *application.App }
)
