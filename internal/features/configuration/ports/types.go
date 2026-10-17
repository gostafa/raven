// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines the configuration loading boundary.
package ports

import (
	"raven/internal/features/configuration/domain"
)

type (
	// Loader reads and validates application settings.
	Loader interface {
		Load() (*domain.Settings, error)
	}
)
