// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package composition defines composition contracts for library.
package composition

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"

	autostart "raven/internal/features/autostart/application"
	companionwails "raven/internal/features/companion/adapters/wails"
	"raven/internal/features/library/adapters/boltstore"
	"raven/internal/features/library/adapters/desktopmedia"
	library "raven/internal/features/library/domain"
	libraryports "raven/internal/features/library/ports"
	reminders "raven/internal/features/reminders/application"
)

type (
	// DatabasePath configures the database owned by this application graph.
	DatabasePath string
	storeParams  struct {
		fx.In

		Batch     libraryports.BatchExecutor
		Lifecycle fx.Lifecycle
		Scheduler library.Scheduler
		Logger    *slog.Logger
		Path      DatabasePath
	}
	storeResult struct {
		fx.Out

		Store      *boltstore.Store
		Repository libraryports.Repository
		Reminders  reminders.Source
		Settings   autostart.Settings
	}
	registryParams struct {
		fx.In

		Batch  libraryports.BatchExecutor
		Store  *boltstore.Store
		Logger *slog.Logger
	}
	mediaParams struct {
		fx.In

		App      *application.App
		Handles  companionwails.WindowSet
		Registry *desktopmedia.Registry
	}
)
