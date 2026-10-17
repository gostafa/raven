// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package wails adapts native file selection and HTTP media previews.
package wails

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
)

type (
	// Settings supplies the media endpoints and log field name.
	Settings struct {
		// MediaRoutePrefix identifies persisted library media requests.
		MediaRoutePrefix string
		// PreviewRoutePrefix identifies temporary selected-file previews.
		PreviewRoutePrefix string
		// LogErrorKey names the structured field used for failures.
		LogErrorKey string
	}

	// Picker selects native files and issues preview capabilities after complete validation.
	Picker struct {
		choose  func(string, string) ([]string, error)
		inspect func([]string, string) (json.RawMessage, error)
	}

	// Registry binds HTTP media routes to a shared preview capability store.
	Registry struct {
		middleware func(http.Handler) http.HandlerFunc
		inspect    func([]string, string) (json.RawMessage, error)
	}

	mediaState struct {
		runBatch           func(context.Context, []func(context.Context) error) error
		saved              func() http.Handler
		logger             *slog.Logger
		mediaRoutePrefix   string
		previewRoutePrefix string
		logErrorKey        string
		previews           sync.Map
	}
)
