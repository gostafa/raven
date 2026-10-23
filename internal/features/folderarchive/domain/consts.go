// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package domain

const (
	zero = 0
	// Version identifies the current portable archive schema.
	Version = 1
	// Fresh initializes imported items as newly created content.
	Fresh = "fresh"
	// Resume restores original schedules and review histories.
	Resume = "resume"
	// ManifestName is the archive's JSON document.
	ManifestName = "cards.json"
	// MaximumManifestSize bounds memory used to decode card metadata.
	MaximumManifestSize = 64 << 20
	// MaximumMediaSize bounds one expanded media file.
	MaximumMediaSize = 8 << 30
	// MaximumArchiveSize bounds total expanded archive data.
	MaximumArchiveSize = 32 << 30
	// MaximumEntries bounds archive directory and record processing.
	MaximumEntries = 100000
)
