// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package main

import (
	"embed"

	"go.uber.org/fx"

	autostartcomposition "raven/internal/features/autostart/composition"
	companioncomposition "raven/internal/features/companion/composition"
	configurationcomposition "raven/internal/features/configuration/composition"
	desktopcomposition "raven/internal/features/desktop/composition"
	desktopports "raven/internal/features/desktop/ports"
	librarycomposition "raven/internal/features/library/composition"
	remindercomposition "raven/internal/features/reminders/composition"
)

var (
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
			fx.Provide(newService),
			configurationcomposition.Module,
			librarycomposition.Module,
			autostartcomposition.Module,
			companioncomposition.Module,
			remindercomposition.Module,
			desktopcomposition.Module,
		),
	)
)
