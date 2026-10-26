// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package otter

import (
	"context"
	"errors"
	"fmt"
	"sync"

	ottercache "github.com/maypok86/otter/v2"
)

// New constructs a dormant cache. Calls before Start or after Close bypass storage.
func New[Key comparable, Value any](options *Options[Key, Value]) *Cache[Key, Value] {
	return &Cache[Key, Value]{
		state: &cacheState[Key, Value]{options: *options, mu: sync.RWMutex{}, cache: nil},
	}
}

// Start initializes a bounded cache without background refresh or expiration.
func (cache *Cache[Key, Value]) Start() error {
	cache.state.mu.Lock()
	defer cache.state.mu.Unlock()

	value, err := ottercache.New(cache.vendorOptions())
	if err != nil {
		return fmt.Errorf("initialize loading cache: %w", err)
	}

	cache.release()

	cache.state.cache = value

	return nil
}

// Close drains active loads before releasing retained entries and maintenance workers.
func (cache *Cache[Key, Value]) Close() {
	cache.state.mu.Lock()
	defer cache.state.mu.Unlock()

	cache.release()
}

func (cache *Cache[Key, Value]) release() {
	if cache.state.cache != nil {
		cache.state.cache.StopAllGoroutines()
		cache.state.cache.InvalidateAll()

		cache.state.cache = nil
	}
}

// Get returns a cached value or shares one synchronous load among concurrent callers.
func (cache *Cache[Key, Value]) Get(
	ctx context.Context, key Key, load func(context.Context) (Value, error),
) (Value, error) {
	cache.state.mu.RLock()
	defer cache.state.mu.RUnlock()

	if cache.state.cache == nil {
		value, err := load(ctx)

		return value, errors.Join(err)
	}

	value, err := cache.state.cache.Get(ctx, key, ottercache.LoaderFunc[Key, Value](
		func(ctx context.Context, _ Key) (Value, error) { return cache.load(ctx, key, load) },
	))
	if errors.Is(err, errOversized) {
		return value, nil
	}

	return value, errors.Join(err)
}

func (cache *Cache[Key, Value]) load(ctx context.Context, key Key,
	load func(context.Context) (Value, error),
) (Value, error) {
	value, err := load(ctx)
	if err != nil {
		return value, errors.Join(err)
	}

	if cache.oversized(key, value) {
		return value, errOversized
	}

	return value, nil
}

func (cache *Cache[Key, Value]) oversized(key Key, value Value) bool {
	if cache.state.options.Weigher == nil {
		return false
	}

	return uint64(cache.state.options.Weigher(key, value)) > cache.state.options.MaximumWeight
}

// InvalidateAll discards retained values; callers synchronize this with their writes.
func (cache *Cache[Key, Value]) InvalidateAll() {
	cache.state.mu.RLock()
	defer cache.state.mu.RUnlock()

	if cache.state.cache != nil {
		cache.state.cache.InvalidateAll()
	}
}

func (cache *Cache[Key, Value]) vendorOptions() *ottercache.Options[Key, Value] {
	var options ottercache.Options[Key, Value]

	options.MaximumSize = cache.state.options.MaximumSize
	options.MaximumWeight = cache.state.options.MaximumWeight
	options.Weigher = cache.state.options.Weigher

	return &options
}
