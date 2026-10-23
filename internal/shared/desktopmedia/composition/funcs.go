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
		params.Batch,
		&desktopmedia.Dependencies{
			Saved:  func() http.Handler { return params.Store.MediaHandler() },
			Logger: params.Logger,
		},
		&desktopmedia.Settings{
			MediaRoutePrefix:   settings.MediaRoutePrefix(),
			PreviewRoutePrefix: settings.PreviewRoutePrefix(),
			LogErrorKey:        settings.LogErrorKey(),
		},
	)
}

func newSelection(params mediaParams) *desktopmedia.Picker {
	return desktopmedia.NewLocalizedPicker(
		params.App,
		params.Handles.Main,
		desktopmedia.NewPickerOptions(params.Registry, params.Messages),
	)
}

func newArchiveDialogs(params mediaParams) *desktopmedia.ArchiveDialogs {
	return desktopmedia.NewArchiveDialogs(params.App, params.Handles.Main, params.Messages)
}
