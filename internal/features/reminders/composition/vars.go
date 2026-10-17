// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	reminderwails "raven/internal/features/reminders/adapters/wails"
	"raven/internal/features/reminders/ports"
)

// Module owns optional notifications and desktop-ready reconciliation.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module("reminders", fx.Provide(
	fx.Annotate(newNative, fx.As(new(reminderwails.NativeRuntime))),
	fx.Annotate(newNotifications, fx.As(new(ports.Notifications)), fx.As(fx.Self())),
	fx.Annotate(newWorker, fx.As(new(ports.Control))),
	newCallbacks,
))
