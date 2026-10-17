// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"go.uber.org/fx"

	configuration "raven/internal/features/configuration/domain"
	"raven/internal/features/library/adapters/boltstore"
	"raven/internal/features/library/adapters/desktopmedia"
	"raven/internal/features/library/adapters/scheduling"
)

func newStore(params storeParams, settings *configuration.Settings) storeResult {
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

func newRegistry(
	store *boltstore.Store,
	logger *slog.Logger,
	settings *configuration.Settings,
) *desktopmedia.Registry {
	return desktopmedia.NewRegistry(
		func() http.Handler { return store.MediaHandler() },
		logger,
		&desktopmedia.Settings{
			MediaRoutePrefix:   settings.MediaRoutePrefix,
			PreviewRoutePrefix: settings.PreviewRoutePrefix,
			LogErrorKey:        settings.LogErrorKey,
		},
	)
}

func newSelection(params mediaParams) *desktopmedia.Picker {
	return desktopmedia.NewPicker(params.App, params.Handles.Main, params.Registry)
}

func newScheduler(settings *configuration.Settings) scheduling.Scheduler {
	return scheduling.New(&settings.Library)
}

func storeDependencies(
	params storeParams,
	settings *configuration.Settings,
) *boltstore.Dependencies {
	return &boltstore.Dependencies{
		Scheduler: params.Scheduler, Logger: params.Logger,
		Settings: boltstore.Settings{
			Rules:            settings.Library,
			MediaRoutePrefix: settings.MediaRoutePrefix,
			LogErrorKey:      settings.LogErrorKey,
		},
	}
}
