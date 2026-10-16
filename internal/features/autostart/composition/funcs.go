// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	startupwails "raven/internal/features/autostart/adapters/wails"
	autostart "raven/internal/features/autostart/application"
)

func newLaunch(params launchParams) launchResult {
	visibility := params.Requests.Visibility()
	params.Detector.PrepareLoginDetection(func(automatic bool) {
		// Cocoa callbacks must not wait for the UI thread.
		go visibility.Classify(automatic || autostart.IsAutostartLaunch(os.Args[1:]))
	})

	return launchResult{Visibility: visibility, Requests: params.Requests}
}

func newController(app *application.App) *autostart.Controller {
	return autostart.New(startupwails.New(app.Autostart), app.Env.Info().Debug, nil)
}
