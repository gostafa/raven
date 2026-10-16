// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	"raven/internal/features/autostart/adapters/launch"
	"raven/internal/features/autostart/adapters/native"
	"raven/internal/features/autostart/ports"
)

var Module = fx.Module("autostart", fx.Provide(
	launch.New,
	fx.Annotate(native.New, fx.As(new(ports.LoginDetection))),
	newLaunch,
	newController,
))
