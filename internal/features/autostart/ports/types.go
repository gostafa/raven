// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines native login launch detection.
package ports

type (
	// LoginDetection registers the callback that classifies an operating system login launch.
	LoginDetection interface {
		PrepareLoginDetection(detected func(bool))
	}
)
