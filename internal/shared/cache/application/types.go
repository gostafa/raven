// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package application reads bounded media chunks without retaining file handles.
package application

import (
	"io"

	"raven/internal/shared/cache/ports"
)

type (
	// Source describes one already-authorized, open file version.
	Source = struct {
		Retry   ports.ReadRetry
		Cache   ports.Chunks
		File    io.ReaderAt
		Version string
		Size    int64
	}
	// Reader owns only a request-local cursor; its caller owns the source lifetime.
	Reader struct {
		state *readerState
	}
	readerState = struct {
		load   func(int64) ([]byte, error)
		chunk  []byte
		start  int64
		size   int64
		offset int64
	}
)
