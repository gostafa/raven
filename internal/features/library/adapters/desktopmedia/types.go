// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package desktopmedia adapts native file selection and HTTP media previews.
package desktopmedia

import (
	"log/slog"
	"net/http"
	"sync"

	"raven/internal/features/library/ports"
)

type (
	// Dependencies supplies the media registry's stable runtime dependencies.
	Dependencies struct {
		Saved  func() http.Handler
		Logger *slog.Logger
		Batch  ports.BatchExecutor
	}

	// Settings supplies the media endpoints and log field name.
	Settings struct {
		MediaRoutePrefix   string
		PreviewRoutePrefix string
		LogErrorKey        string
	}

	// Picker owns file selection and the preview capabilities issued to selected paths.
	Picker struct {
		*Registry

		choose func(kind, filter string) ([]string, error)
	}

	// Registry owns HTTP media routes and preview capabilities without native dependencies.
	Registry struct {
		batch    ports.BatchExecutor
		settings Settings
		saved    func() http.Handler
		logger   *slog.Logger
		previews sync.Map
	}
)
