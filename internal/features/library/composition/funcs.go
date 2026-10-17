// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"go.uber.org/fx"

	"raven/internal/features/library/adapters/boltstore"
	"raven/internal/features/library/adapters/desktopmedia"
)

func newStore(params storeParams) storeResult {
	store := boltstore.New(string(params.Path), params.Scheduler, params.Logger)
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

func newRegistry(store *boltstore.Store, logger *slog.Logger) *desktopmedia.Registry {
	return desktopmedia.NewRegistry(func() http.Handler { return store.MediaHandler() }, logger)
}

func newSelection(params mediaParams) *desktopmedia.Picker {
	return desktopmedia.NewPicker(params.App, params.Handles.Main, params.Registry)
}
