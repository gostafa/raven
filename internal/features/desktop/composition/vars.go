// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"

	desktopwails "raven/internal/features/desktop/adapters/wails"
	"raven/internal/features/desktop/ports"
	"raven/internal/shared/lifetime"
)

var (
	_      application.ServiceStartup  = (*Bridge)(nil)
	_      application.ServiceShutdown = (*Bridge)(nil)
	_      Lifecycle                   = (*fx.App)(nil)
	Module                             = fx.Module("desktop",
		fx.Provide(
			lifetime.New,
			newApplication,
			desktopwails.NewWindows,
			fx.Annotate(desktopwails.NewEvents, fx.As(new(ports.Events))),
			newDesktop,
		),
		fx.Invoke(configureDesktop),
	)
)
