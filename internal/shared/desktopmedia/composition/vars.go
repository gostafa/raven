// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	mediaports "raven/internal/shared/desktopmedia/ports"
)

// Module wires media selection and routing.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module(
	"desktopmedia",
	fx.Provide(
		fx.Annotate(newRegistry, fx.As(new(mediaports.MediaRoutes)), fx.As(fx.Self())),
		fx.Annotate(newSelection, fx.As(new(mediaports.Selection))),
	),
)
