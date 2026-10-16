// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	"raven/internal/features/reminders/ports"
)

var Module = fx.Module("reminders", fx.Provide(
	newNative,
	fx.Annotate(newNotifications, fx.As(new(ports.Notifications)), fx.As(fx.Self())),
	fx.Annotate(newWorker, fx.As(new(ports.Control))),
	newCallbacks,
))
