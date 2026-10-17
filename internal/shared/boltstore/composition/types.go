// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package composition wires database persistence and its lifecycle.
package composition

import (
	"log/slog"

	"go.uber.org/fx"

	autostart "raven/internal/features/autostart/application"
	reminders "raven/internal/features/reminders/application"
	boltstore "raven/internal/shared/boltstore/adapters/bbolt"
	libraryports "raven/internal/shared/boltstore/ports"
)

type (
	// DatabasePath configures the database owned by this application graph.
	DatabasePath string
	storeParams  struct {
		fx.In

		Batch     libraryports.BatchExecutor
		Lifecycle fx.Lifecycle
		Scheduler libraryports.Scheduler
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
)
