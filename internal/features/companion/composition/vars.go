// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	companion "raven/internal/features/companion/domain"
	"raven/internal/features/companion/ports"
)

// Module wires native window operations to the companion ports.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module("companion", fx.Provide(
	fx.Annotate(newConfiguration, fx.As(new(ports.WidgetConfiguration))),
	fx.Annotate(newWindows, fx.As(new(companion.Windows))),
))
