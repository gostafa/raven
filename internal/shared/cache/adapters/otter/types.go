// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package otter adapts bounded loading caches without exposing vendor contracts.
package otter

import (
	"sync"

	ottercache "github.com/maypok86/otter/v2"
)

type (
	// Options sets either an entry limit or a byte-weighted capacity.
	Options[Key comparable, Value any] = struct {
		Weigher       func(Key, Value) uint32
		MaximumSize   int
		MaximumWeight uint64
	}
	// Cache allocates maintenance resources only when started by its owner.
	Cache[Key comparable, Value any] struct {
		state *cacheState[Key, Value]
	}
	cacheState[Key comparable, Value any] = struct {
		cache   *ottercache.Cache[Key, Value]
		options Options[Key, Value]
		mu      sync.RWMutex
	}
)
