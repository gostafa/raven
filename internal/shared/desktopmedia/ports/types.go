// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines media selection, routing, and batch boundaries.
package ports

import (
	"context"
	"net/http"

	mediaapi "raven/api/media"
)

type (
	// BatchExecutor runs independent jobs with bounded concurrency. It returns the
	// first error in input order and drains started jobs even after cancellation.
	// Job failures do not skip other jobs; cancellation skips pending jobs.
	// Panics become errors, and an empty batch succeeds even when canceled.
	BatchExecutor interface {
		Run(ctx context.Context, jobs []func(context.Context) error) error
	}

	// Media selects local files and serves temporary or saved media.
	Media interface {
		Selection
		MediaRoutes
	}

	// Selection selects local files for a draft.
	Selection interface {
		Select(kind string) ([]mediaapi.Selection, error)
	}

	// MediaRoutes serves previews and persisted content independently of native windows.
	MediaRoutes interface {
		Middleware(next http.Handler) http.HandlerFunc
	}
)
