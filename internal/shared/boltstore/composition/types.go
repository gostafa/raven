// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package composition wires database persistence and its lifecycle.
package composition

import (
	"log/slog"

	"go.uber.org/fx"

	autostartports "raven/internal/features/autostart/ports"
	reminders "raven/internal/features/reminders/application"
	boltstore "raven/internal/shared/boltstore/adapters/bbolt"
	libraryports "raven/internal/shared/boltstore/ports"
	cacheports "raven/internal/shared/cache/ports"
	localizationports "raven/internal/shared/localization/ports"
)

type (
	// DatabasePath configures the database owned by this application graph.
	DatabasePath string
	storeParams  struct {
		fx.In

		Chunks      cacheports.Chunks
		Snapshots   cacheports.Loading[string, string]
		MediaLookup cacheports.Loading[string, libraryports.MediaRecord]

		Batch     libraryports.BatchExecutor
		Lifecycle fx.Lifecycle
		Scheduler libraryports.Scheduler
		Logger    *slog.Logger
		Path      DatabasePath
	}
	storeResult struct {
		fx.Out

		Languages  localizationports.Preferences
		Store      *boltstore.Store
		Repository libraryports.Repository
		Reminders  reminders.Source
		Settings   autostartports.Settings
	}
)
