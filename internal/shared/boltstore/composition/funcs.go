// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	configuration "raven/internal/features/configuration/domain"
	boltstore "raven/internal/shared/boltstore/adapters/bbolt"
)

func newStore(params storeParams, settings configuration.Settings) storeResult {
	store := boltstore.New(string(params.Path), storeDependencies(params, settings))
	params.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error { return store.Start() },
		OnStop:  func(context.Context) error { return store.Close() },
	})

	return storeResult{
		Out:        fx.Out{},
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
	return &boltstore.Dependencies{
		Batch: params.Batch, Scheduler: params.Scheduler, Logger: params.Logger,
		Settings: boltstore.Settings{
			Rules:            settings.Library(),
			MediaRoutePrefix: settings.MediaRoutePrefix(),
			LogErrorKey:      settings.LogErrorKey(),
		},
	}
}
