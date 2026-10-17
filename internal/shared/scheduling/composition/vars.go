// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	storeports "raven/internal/shared/boltstore/ports"
)

// Module wires spaced repetition scheduling.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module(
	"scheduling",
	fx.Provide(fx.Annotate(newScheduler, fx.As(new(storeports.Scheduler)))),
)
