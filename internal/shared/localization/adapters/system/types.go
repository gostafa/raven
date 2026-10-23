// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package system reads the user's operating-system languages.
package system

type (
	// Locales reads fresh language preferences on every request.
	Locales struct{ read func() ([]string, error) }
)
