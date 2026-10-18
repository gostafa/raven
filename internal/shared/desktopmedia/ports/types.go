// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines media selection, routing, and batch boundaries.
package ports

import (
	"context"
	"net/http"
)

type (
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
)
