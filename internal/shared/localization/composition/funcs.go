// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"context"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"

	api "raven/api/localization"
	localization "raven/internal/shared/localization/application"
)

func newService(params serviceParams) *localization.Service {
	service := localization.New(params.Preferences, params.System, func(err error) {
		params.Logger.WarnContext(
			context.Background(),
			"language preferences unavailable",
			slog.Any(languageErrorKey, err),
		)
	})
	params.Lifecycle.Append(
		fx.Hook{
			OnStart: func(context.Context) error {
				service.Initialize()

				return nil
			},
			OnStop: nil,
		},
	)

	return service
}

func publishChanges(lifecycle fx.Lifecycle, service *localization.Service, app *application.App) {
	cancel := service.Subscribe(func() { publishLanguage(app, service) })

	lifecycle.Append(

		fx.Hook{OnStart: nil, OnStop: func(context.Context) error {
			cancel()

			return nil
		}},
	)
}

func publishLanguage(app *application.App, service *localization.Service) {
	state := service.Current()
	app.Event.Emit(
		"raven:localization-changed",
		api.State{
			Preference: state.Preference,
			Locale:     state.Locale,
			Direction:  state.Direction,
			Revision:   state.Revision,
		},
	)
}
