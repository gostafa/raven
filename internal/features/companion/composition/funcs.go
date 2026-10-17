// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"raven/internal/features/companion/adapters/native"
	"raven/internal/features/companion/adapters/wails"
	configuration "raven/internal/features/configuration/domain"
)

func newConfiguration(handles wails.WindowSet) native.Widget {
	return native.New(handles.Widget)
}

func newWindows(params windowParams, settings *configuration.Settings) *wails.Windows {
	return wails.New(
		&wails.Dependencies{
			App:           params.App,
			Handles:       params.Handles,
			Configuration: params.Configuration,
			Settings: wails.Settings{
				Layout:              settings.Layout,
				LogErrorKey:         settings.LogErrorKey,
				OpenFloatingReview:  settings.OpenFloatingReview,
				CloseFloatingReview: settings.CloseFloatingReview,
			},
		},
	)
}
