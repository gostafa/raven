// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package lifetime

import (
	"context"
	"sync"
)

type (
	// Runtime bounds background work to the desktop service lifetime.
	Runtime struct {
		ctx    context.Context
		cancel context.CancelFunc
		detach func() bool
		mu     sync.Mutex
	}
)
