// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	library "raven/internal/features/library/domain"
	libraryports "raven/internal/features/library/ports"
)

// Module wires persistence, scheduling, and native media selection.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module("library",
	fx.Provide(
		fx.Annotate(newScheduler, fx.As(new(library.Scheduler))),
		newStore,
		defaultPath,
		fx.Annotate(newRegistry, fx.As(new(libraryports.MediaRoutes)), fx.As(fx.Self())),
		fx.Annotate(newSelection, fx.As(new(libraryports.Selection))),
	),
)
