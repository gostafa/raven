// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package composition defines composition contracts for desktop.
package composition

import (
	"context"
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"

	autostart "raven/internal/features/autostart/application"
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
		Visibility *autostart.Visibility
		Logger     *slog.Logger
		Assets     ports.Assets
	}
	desktopParams struct {
		fx.In

		Handles       companionwails.WindowSet
		Reminders     reminderports.Control
		Lifecycle     fx.Lifecycle
		Requests      ports.OpenRequests
		Windows       companion.Windows
		Settings      autostart.Settings
		Visibility    *autostart.Visibility
		Startup       *autostart.Controller
		Notifications *reminderwails.Runtime
		Runtime       *lifetime.Runtime
		App           *application.App
		Assets        ports.Assets
	}
	// Lifecycle is the graph control boundary used by the Wails bridge.
	Lifecycle interface {
		Start(ctx context.Context) error
		Stop(ctx context.Context) error
	}
	// Bridge delegates Wails service lifecycle to Fx exactly once.
	Bridge struct {
		graph    Lifecycle
		startErr error
		stopErr  error
		runtime  *lifetime.Runtime
		start    sync.Once
		stop     sync.Once
	}
	lifecycleError struct{ message string }
)
