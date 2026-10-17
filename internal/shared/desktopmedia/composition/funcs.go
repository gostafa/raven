// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"net/http"

	configuration "raven/internal/features/configuration/domain"
	desktopmedia "raven/internal/shared/desktopmedia/adapters/wails"
)

func newRegistry(params registryParams, settings configuration.Settings) *desktopmedia.Registry {
	return desktopmedia.NewRegistry(
		&desktopmedia.Dependencies{
			Saved:  func() http.Handler { return params.Store.MediaHandler() },
			Logger: params.Logger,
			Batch:  params.Batch,
		},
		&desktopmedia.Settings{
			MediaRoutePrefix:   settings.MediaRoutePrefix(),
			PreviewRoutePrefix: settings.PreviewRoutePrefix(),
			LogErrorKey:        settings.LogErrorKey(),
		},
	)
}

func newSelection(params mediaParams) *desktopmedia.Picker {
	return desktopmedia.NewPicker(params.App, params.Handles.Main, params.Registry)
}
