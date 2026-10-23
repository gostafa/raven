// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package composition defines composition contracts for desktop.
package composition

import (
	"context"
	"io/fs"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"

	companionwails "raven/internal/features/companion/adapters/wails"
	companion "raven/internal/features/companion/domain"
	"raven/internal/features/desktop/ports"
	reminderwails "raven/internal/features/reminders/adapters/wails"
	reminderports "raven/internal/features/reminders/ports"
	mediaports "raven/internal/shared/desktopmedia/ports"
	"raven/internal/shared/lifetime"
)

type (
	appParams struct {
		fx.In

		Media      mediaports.MediaRoutes
		Visibility ports.LaunchVisibility
		Logger     *slog.Logger
		Assets     Assets
	}
	desktopParams struct {
		fx.In

		Localization  ports.Localization
		Handles       companionwails.WindowSet
		Reminders     reminderports.Control
		Lifecycle     fx.Lifecycle
		Requests      ports.OpenRequests
		Windows       companion.Windows
		Visibility    ports.LaunchVisibility
		Startup       ports.Startup
		Notifications *reminderwails.Runtime
		Runtime       *lifetime.Runtime
		App           *application.App
		Assets        Assets
	}
	// Lifecycle is the graph control boundary used by the Wails bridge.
	Lifecycle interface {
		Start(ctx context.Context) error
		Stop(ctx context.Context) error
	}
	// Bridge delegates Wails service lifecycle to Fx exactly once.
	Bridge struct {
		lifecycle func(context.Context, bool) error
	}
	lifecycleError struct{ message string }

	// Assets supplies embedded desktop files and the application icon.
	Assets struct {
		Files fs.FS
		Icon  []byte
	}
)
