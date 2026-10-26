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

	"github.com/wailsapp/wails/v3/pkg/application"

	cacheports "raven/internal/shared/cache/ports"
	"raven/internal/shared/desktopmedia/ports"
)

type (
	chooserOptions = struct {
		prompt   func(*application.OpenFileDialogStruct) ([]string, error)
		messages []ports.Messages
	}
	archiveDialogState = struct {
		messages ports.Messages
		app      *application.App
		window   *application.WebviewWindow
		save     func(*application.SaveFileDialogStruct) (string, error)
		open     func(*application.OpenFileDialogStruct) (string, error)
	}

	// ArchiveDialogs selects portable archive files using Raven's main window.
	ArchiveDialogs struct{ state *archiveDialogState }
	// Dependencies supplies saved-media resolution and resource cleanup logging.
	Dependencies = struct {
		// Chunks shares payload storage with persisted media.
		Chunks cacheports.Chunks
		// Retry bounds temporary preview read failures.
		Retry cacheports.ReadRetry
		// Saved resolves the handler for persisted library media.
		Saved func() http.Handler
		// Logger receives failures encountered while releasing preview resources.
		Logger *slog.Logger
	}

	previewRequest = struct {
		retry   cacheports.ReadRetry
		chunks  cacheports.Chunks
		writer  http.ResponseWriter
		request *http.Request
		path    string
	}

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

	mediaState = struct {
		retry              cacheports.ReadRetry
		chunks             cacheports.Chunks
		runBatch           func(context.Context, []func(context.Context) error) error
		saved              func() http.Handler
		logger             *slog.Logger
		mediaRoutePrefix   string
		previewRoutePrefix string
		logErrorKey        string
		previews           sync.Map
	}
)
