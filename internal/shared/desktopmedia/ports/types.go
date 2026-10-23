// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines media selection, routing, and batch boundaries.
package ports

import (
	"context"
	"encoding/json"
	"net/http"
)

type (
	// PickerOptions supplies preview inspection and localized file filters.
	PickerOptions = struct {
		Inspect  func([]string, string) (json.RawMessage, error)
		Messages Messages
	}
	// BatchExecutor runs independent jobs with bounded concurrency. It returns the
	// first error in input order and drains started jobs even after cancellation.
	// Job failures do not skip other jobs; cancellation skips pending jobs.
	// Panics become errors, and an empty batch succeeds even when canceled.
	BatchExecutor interface {
		Run(ctx context.Context, jobs []func(context.Context) error) error
	}

	// Selection returns a JSON array of selected local files and preview capabilities.
	Selection interface {
		Select(kind string) (string, error)
	}

	// MediaRoutes serves previews and persisted content independently of native windows.
	MediaRoutes interface {
		Middleware(next http.Handler) http.HandlerFunc
	}

	// Messages supplies localized native text without exposing a translation library.
	Messages interface {
		Text(key string, count int) string
	}
)
