// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package lifetime

import (
	"context"
)

// New allocates a context without starting background work.
func New() *Runtime {
	ctx, cancel := context.WithCancel(context.Background())

	var runtime Runtime

	runtime.ctx = ctx
	runtime.cancel = cancel

	return &runtime
}

// Context returns the context used by managed workers.
func (runtime *Runtime) Context() context.Context { return runtime.ctx }

// Attach propagates cancellation from the Wails service context.
func (runtime *Runtime) Attach(parent context.Context) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.detach = context.AfterFunc(parent, runtime.cancel)
}

// Cancel ends background work and releases the parent subscription.
func (runtime *Runtime) Cancel() {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.detach != nil {
		runtime.detach()
	}

	runtime.cancel()
}
