// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package desktopmedia adapts native file selection and HTTP media previews.
package desktopmedia

import (
	"log/slog"
	"net/http"
	"sync"
)

type (
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
		settings Settings
		saved    func() http.Handler
		logger   *slog.Logger
		previews sync.Map
	}
)
