// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"

	startupwails "raven/internal/features/autostart/adapters/wails"
	autostart "raven/internal/features/autostart/application"
	autostartports "raven/internal/features/autostart/ports"
	configuration "raven/internal/features/configuration/domain"
)

func newLaunch(params launchParams) launchResult {
	visibility := params.Requests.Visibility()
	autostart.PrepareLaunch(params.Detector, visibility, os.Args[1:])

	return launchResult{Out: fx.Out{}, Visibility: visibility, Requests: params.Requests}
}

func newController(
	app *application.App,
	settings configuration.Settings,
	initialization autostartports.Settings,
) *autostart.Controller {
	return autostart.New(
		startupwails.New(app.Autostart),
		initialization,
		&autostart.Configuration{
			Debug:         app.Env.Info().Debug,
			ApplicationID: settings.ApplicationID(),
		},
	)
}
