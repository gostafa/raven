// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"context"
	"log/slog"

	"go.uber.org/fx"

	cacheotter "raven/internal/shared/cache/adapters/otter"
	resilience "raven/internal/shared/resilience/adapters/retry"
)

func newChunks(lifecycle fx.Lifecycle) *cacheotter.Cache[string, []byte] {
	return Register(lifecycle, &cacheotter.Options[string, []byte]{
		MaximumSize:   0,
		MaximumWeight: mediaCapacity,
		Weigher:       func(_ string, value []byte) uint32 { return uint32(min(len(value), mediaCapacity+1)) },
	})
}

// Register gives a cache lifecycle ownership before consumers acquire their resources.
func Register[Key comparable, Value any](lifecycle fx.Lifecycle,
	options *cacheotter.Options[Key, Value],
) *cacheotter.Cache[Key, Value] {
	cache := cacheotter.New(options)

	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error { return cache.Start() },

		OnStop: func(context.Context) error {
			cache.Close()

			return nil
		},
	})

	return cache
}

func newReadRetry(lifecycle fx.Lifecycle, logger *slog.Logger) *resilience.Reads {
	ctx, cancel := context.WithCancel(context.Background())

	lifecycle.Append(fx.Hook{OnStart: nil, OnStop: func(context.Context) error {
		cancel()

		return nil
	}})

	return resilience.NewReads(ctx, logger)
}
