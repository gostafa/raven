// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	libraryports "raven/internal/shared/boltstore/ports"
	cachecomposition "raven/internal/shared/cache/composition"
	cacheports "raven/internal/shared/cache/ports"
)

// Module wires persistence and database lifecycle.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module("boltstore", cachecomposition.Module, fx.Provide(newStore, defaultPath,
	fx.Annotate(newSnapshots, fx.As(new(cacheports.Loading[string, string]))),
	fx.Annotate(newMediaLookup, fx.As(new(cacheports.Loading[string, libraryports.MediaRecord]))),
))
