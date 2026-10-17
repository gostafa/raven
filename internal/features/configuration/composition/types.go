// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package composition wires the embedded configuration adapter.
package composition

import (
	"go.uber.org/fx"

	"raven/internal/features/configuration/domain"
)

type (
	settingsResult struct {
		fx.Out

		Settings domain.Settings
	}
)
