// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package main

import (
	"embed"

	"go.uber.org/fx"

	autostartcomposition "raven/internal/features/autostart/composition"
	companioncomposition "raven/internal/features/companion/composition"
	configurationcomposition "raven/internal/features/configuration/composition"
	desktopwails "raven/internal/features/desktop/adapters/wails"
	desktopcomposition "raven/internal/features/desktop/composition"
	desktopports "raven/internal/features/desktop/ports"
	remindercomposition "raven/internal/features/reminders/composition"
	storecomposition "raven/internal/shared/boltstore/composition"
	batchcomposition "raven/internal/shared/cffbatch/composition"
	mediacomposition "raven/internal/shared/desktopmedia/composition"
	schedulingcomposition "raven/internal/shared/scheduling/composition"
)

var (
	_ desktopRunner = (*desktopwails.Desktop)(nil)
	//go:embed all:frontend/dist
	assets embed.FS
	//go:embed build/appicon.png
	appIcon []byte
	//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
	applicationModule = fx.Options(
		fx.Provide(newLogger),
		fx.WithLogger(newFxLogger),
		fx.Module("raven",
			fx.Supply(desktopports.Assets{Files: assets, Icon: appIcon}),
			fx.Provide(newService, fx.Annotate(desktopRuntime, fx.As(new(desktopRunner)))),
			configurationcomposition.Module,
			storecomposition.Module,
			mediacomposition.Module,
			schedulingcomposition.Module,
			batchcomposition.Module,
			autostartcomposition.Module,
			companioncomposition.Module,
			remindercomposition.Module,
			desktopcomposition.Module,
		),
	)
)
