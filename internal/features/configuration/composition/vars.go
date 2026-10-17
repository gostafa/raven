// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"
)

// Module loads the embedded defaults once for the application graph.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module(
	"configuration",
	fx.Provide(newLoader, load),
)
