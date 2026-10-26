// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	transferapplication "raven/internal/features/cardtransfer/application"
	"raven/internal/features/cardtransfer/ports"
)

// Module wires card transfer workflows to transactional persistence.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module("cardtransfer", fx.Provide(
	fx.Annotate(transferLibrary, fx.As(new(ports.Library))),
	fx.Annotate(transferapplication.New, fx.As(new(ports.Transfer))),
))
