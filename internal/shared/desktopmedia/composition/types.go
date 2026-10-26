// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package composition wires native media selection and HTTP previews.
package composition

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"

	companionwails "raven/internal/features/companion/adapters/wails"
	boltstore "raven/internal/shared/boltstore/adapters/bbolt"
	cacheports "raven/internal/shared/cache/ports"
	desktopmedia "raven/internal/shared/desktopmedia/adapters/wails"
	mediaports "raven/internal/shared/desktopmedia/ports"
)

type (
	registryParams struct {
		fx.In

		Chunks cacheports.Chunks
		Retry  cacheports.ReadRetry

		Batch  mediaports.BatchExecutor
		Store  *boltstore.Store
		Logger *slog.Logger
	}
	mediaParams struct {
		fx.In

		Messages mediaports.Messages
		App      *application.App
		Handles  companionwails.WindowSet
		Registry *desktopmedia.Registry
	}
)
