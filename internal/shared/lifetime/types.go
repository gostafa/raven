// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package lifetime defines lifetime contracts for shared.
package lifetime

import (
	"context"
	"sync"
)

type (
	// Runtime bounds background work to the desktop service lifetime.
	Runtime struct {
		ctx    context.Context //nolint:containedctx // This type owns and cancels the application lifetime.
		cancel context.CancelFunc
		detach func() bool
		mu     sync.Mutex
	}
)
