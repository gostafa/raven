// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	storeports "raven/internal/shared/boltstore/ports"
	cffbatch "raven/internal/shared/cffbatch/adapters/cff"
	mediaports "raven/internal/shared/desktopmedia/ports"
)

// Module wires one executor to both consumer batch ports.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module(
	"cffbatch",
	fx.Provide(
		fx.Annotate(
			cffbatch.New,
			fx.As(new(storeports.BatchExecutor)),
			fx.As(new(mediaports.BatchExecutor)),
		),
	),
)
