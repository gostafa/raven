// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"io/fs"

	"go.uber.org/fx"

	desktop "raven/internal/features/desktop/ports"
	reminders "raven/internal/features/reminders/ports"
	media "raven/internal/shared/desktopmedia/ports"
	"raven/internal/shared/localization/adapters/native"
	"raven/internal/shared/localization/adapters/system"
	"raven/internal/shared/localization/ports"
)

// Module registers localization without acquiring resources during construction.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registrations.
var Module = fx.Module("localization", fx.Provide(
	fx.Annotate(native.Files, fx.As(new(fs.FS))),
	fx.Annotate(newService, fx.As(new(ports.LocaleSource)), fx.As(fx.Self())),
	fx.Annotate(system.New, fx.As(new(ports.SystemLocales))),
	fx.Annotate(
		native.New,
		fx.As(new(desktop.Localization)),
		fx.As(new(reminders.Messages)),
		fx.As(new(media.Messages)),
	),
), fx.Invoke(publishChanges))
