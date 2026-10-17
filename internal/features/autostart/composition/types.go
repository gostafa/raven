// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package composition defines composition contracts for autostart.
package composition

import (
	"go.uber.org/fx"

	autostart "raven/internal/features/autostart/application"
	autostartports "raven/internal/features/autostart/ports"
	desktopports "raven/internal/features/desktop/ports"
)

type (
	launchResult struct {
		fx.Out

		Visibility desktopports.LaunchVisibility
		Requests   desktopports.OpenRequests
	}
	launchParams struct {
		fx.In

		Detector autostartports.LoginDetection
		Requests *autostart.Requests
	}
)
