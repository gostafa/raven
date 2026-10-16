// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines native floating widget configuration.
package ports

type (
	// WidgetConfiguration applies native styling to the floating widget.
	WidgetConfiguration interface {
		Configure() error
	}
)
