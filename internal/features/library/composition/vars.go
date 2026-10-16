// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	"raven/internal/features/library/adapters/scheduling"
	library "raven/internal/features/library/domain"
	libraryports "raven/internal/features/library/ports"
)

var Module = fx.Module("library",
	fx.Provide(
		fx.Annotate(scheduling.New, fx.As(new(library.Scheduler))),
		newStore,
		defaultPath,
		fx.Annotate(newRegistry, fx.As(new(libraryports.MediaRoutes)), fx.As(fx.Self())),
		newSelection,
	),
)
