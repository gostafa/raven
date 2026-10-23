// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package composition wires localization adapters and startup ordering.
package composition

import (
	"log/slog"

	"go.uber.org/fx"

	"raven/internal/shared/localization/ports"
)

type (
	serviceParams struct {
		fx.In

		Lifecycle   fx.Lifecycle
		Preferences ports.Preferences
		System      ports.SystemLocales
		Logger      *slog.Logger
	}
)
