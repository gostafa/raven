// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	"raven/internal/features/autostart/adapters/launch"
	autostart "raven/internal/features/autostart/application"
	autostartports "raven/internal/features/autostart/ports"
	desktopports "raven/internal/features/desktop/ports"
)

type (
	launchResult struct {
		fx.Out

		Visibility *autostart.Visibility
		Requests   desktopports.OpenRequests
	}
	launchParams struct {
		fx.In

		Detector autostartports.LoginDetection
		Requests *launch.Requests
	}
)
