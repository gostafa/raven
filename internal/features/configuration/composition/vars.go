// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	"raven/internal/features/configuration/ports"
)

// Module loads the embedded defaults once for the application graph.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module(
	"configuration",
	fx.Provide(fx.Annotate(newLoader, fx.As(new(ports.Loader))), load),
)
