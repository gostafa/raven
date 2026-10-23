// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package localization defines Raven's public language contract.
package localization

// State describes the saved preference and its resolved presentation.
type (
	State struct {
		// Preference is system, en, or ar.
		Preference string `json:"preference"`
		// Locale is the resolved supported language.
		Locale string `json:"locale"`
		// Direction is ltr or rtl.
		Direction string `json:"direction"`
		// Revision orders changes within one application lifetime.
		Revision uint64 `json:"revision"`
	}
)
