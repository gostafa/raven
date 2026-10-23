// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package domain defines Raven's language preferences independently of its UI.
package domain

// State is an immutable language snapshot shared by application consumers.
type (
	State = struct {
		Preference string
		Locale     string
		Direction  string
		Revision   uint64
	}
)
