// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines loading caches consumed by media readers and repositories.
package ports

import (
	"context"
)

type (
	// ReadRetry bounds safe loads without exposing the retry implementation.
	ReadRetry interface {
		Do(ctx context.Context, operation func(context.Context) error) error
	}
	// Loading retains successful loads and coalesces simultaneous misses.
	Loading[Key comparable, Value any] interface {
		Get(ctx context.Context, key Key, load func(context.Context) (Value, error)) (Value, error)
		InvalidateAll()
	}
	// Chunks stores immutable media payloads shared by saved files and previews.
	Chunks = Loading[string, []byte]
)
