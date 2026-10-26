// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	configuration "raven/internal/features/configuration/domain"
	boltstore "raven/internal/shared/boltstore/adapters/bbolt"
	libraryports "raven/internal/shared/boltstore/ports"
	cacheotter "raven/internal/shared/cache/adapters/otter"
	cachecomposition "raven/internal/shared/cache/composition"
)

func newStore(params storeParams, settings configuration.Settings) storeResult {
	store := boltstore.New(string(params.Path), storeDependencies(params, settings))
	params.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error { return boltstore.StartContext(ctx, store) },
		OnStop:  func(context.Context) error { return store.Close() },
	})

	return storeResult{
		Out:        fx.Out{},
		Languages:  store,
		Store:      store,
		Repository: store,
		Reminders:  store,
		Settings:   store,
	}
}

func defaultPath() (DatabasePath, error) {
	path, err := boltstore.DefaultPath()
	if err != nil {
		return "", fmt.Errorf("locate library: %w", err)
	}

	return DatabasePath(path), nil
}

func storeDependencies(
	params storeParams,
	settings configuration.Settings,
) *boltstore.Dependencies {
	return func() *boltstore.Dependencies {
		deps := boltstore.NewDependencies(params.Batch, params.Scheduler, &boltstore.Settings{
			Rules:            configuration.Library(settings),
			MediaRoutePrefix: settings.MediaRoutePrefix(),
			LogErrorKey:      settings.LogErrorKey(),
		})

		deps.Logger = params.Logger
		deps.Chunks, deps.Snapshots, deps.MediaLookup = params.Chunks, params.Snapshots, params.MediaLookup

		return deps
	}()
}

func newSnapshots(lifecycle fx.Lifecycle) *cacheotter.Cache[string, string] {
	return cachecomposition.Register(lifecycle, &cacheotter.Options[string, string]{
		MaximumSize:   0,
		MaximumWeight: snapshotCapacity,
		Weigher:       snapshotWeight,
	})
}

func snapshotWeight(_, value string) uint32 {
	return uint32(min(len(value), snapshotCapacity+1))
}

func newMediaLookup(lifecycle fx.Lifecycle) *cacheotter.Cache[string, libraryports.MediaRecord] {
	return cachecomposition.Register(
		lifecycle,
		&cacheotter.Options[string, libraryports.MediaRecord]{
			MaximumSize:   mediaLookupCapacity,
			MaximumWeight: 0, Weigher: nil,
		},
	)
}
