// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	"raven/internal/shared/cache/ports"
)

// Module shares one bounded media cache throughout the application graph.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module("cache", fx.Provide(
	fx.Annotate(newChunks, fx.As(new(ports.Chunks))),
	fx.Annotate(newReadRetry, fx.As(new(ports.ReadRetry))),
))
