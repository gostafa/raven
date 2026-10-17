// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	startupwails "raven/internal/features/autostart/adapters/wails"
	autostart "raven/internal/features/autostart/application"
	"raven/internal/features/autostart/ports"
	desktopports "raven/internal/features/desktop/ports"
)

// Module wires native launch detection and login startup policy.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module("autostart", fx.Provide(
	autostart.NewRequests,
	fx.Annotate(startupwails.NewDetector, fx.As(new(ports.LoginDetection))),
	newLaunch,
	fx.Annotate(newController, fx.As(new(desktopports.Startup))),
))
