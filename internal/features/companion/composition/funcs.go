// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"raven/internal/features/companion/adapters/wails"
	configuration "raven/internal/features/configuration/domain"
)

func newConfiguration(handles wails.WindowSet) wails.Widget {
	return wails.NewWidget(handles.Widget)
}

func newWindows(params windowParams, settings configuration.Settings) *wails.Windows {
	layout := configuration.Layout(settings)

	return wails.New(&wails.Dependencies{
		App: params.App, Main: params.Handles.Main, Widget: params.Handles.Widget,
		Review: params.Handles.Review, Configure: params.Configuration.Configure,
	}, &wails.Settings{
		WidgetSize: layout.WidgetSize, ReviewWidth: layout.ReviewWidth,
		ReviewHeight: layout.ReviewHeight, ReviewGap: layout.ReviewGap,
		LogErrorKey: settings.LogErrorKey(), OpenFloatingReview: settings.OpenFloatingReview(),
		CloseFloatingReview: settings.CloseFloatingReview(),
	})
}
