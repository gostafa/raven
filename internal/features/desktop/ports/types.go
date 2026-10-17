// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines ports contracts for desktop.
package ports

import (
	"io/fs"
)

type (
	// Assets supplies embedded desktop files and the application icon.
	Assets struct {
		Files fs.FS
		Icon  []byte
	}

	// Events publishes frontend notifications.
	Events interface{ Emit(name string) }

	// OpenRequests supplies queued launch visibility effects.
	OpenRequests interface{ Pending() <-chan struct{} }
)
