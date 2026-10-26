// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	desktopports "raven/internal/features/desktop/ports"
	reminderwails "raven/internal/features/reminders/adapters/wails"
	reminders "raven/internal/features/reminders/application"
	"raven/internal/features/reminders/ports"
)

// Module owns optional notifications and desktop-ready reconciliation.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module("reminders", fx.Provide(
	fx.Annotate(newSound, fx.As(new(ports.Sound))),
	fx.Annotate(newNative, fx.As(new(reminderwails.NativeRuntime))),
	fx.Annotate(newNotifications, fx.As(new(reminders.Notifications)), fx.As(fx.Self())),
	fx.Annotate(newApplicationNotifications, fx.As(new(desktopports.Notifications))),
	fx.Annotate(newWorker, fx.As(new(ports.Control))),
	newCallbacks,
))
