// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package composition defines composition contracts for companion.
package composition

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"

	"raven/internal/features/companion/adapters/wails"
	"raven/internal/features/companion/ports"
)

type (
	windowParams struct {
		fx.In

		App           *application.App
		Handles       wails.WindowSet
		Configuration ports.WidgetConfiguration
	}
)
