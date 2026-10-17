// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	companion "raven/internal/features/companion/domain"
	"raven/internal/features/companion/ports"
)

var (
	_ companion.Windows = (*Windows)(nil)

	_ ports.WidgetConfiguration = (*Widget)(nil)
)
