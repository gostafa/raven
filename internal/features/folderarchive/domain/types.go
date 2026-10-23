// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package domain defines the portable folder archive format.
package domain

import (
	"time"

	cardapi "raven/api/cards"
	folderapi "raven/api/folders"
)

type (
	// Document is the versioned cards.json manifest. Media paths are archive-relative.
	Document = struct {
		// ExportedAt is the time the library snapshot was taken.
		ExportedAt time.Time `json:"exported_at"`
		// Folders contains every selected folder, including empty ones.
		Folders []Folder `json:"folders"`
		// Version identifies the portable archive schema.
		Version int `json:"version"`
	}
	// Folder includes folder metadata and every card in that folder.
	Folder = struct {
		// Metadata preserves the original folder metadata.
		Metadata folderapi.Folder `json:"folder"`
		// Cards contains complete card content and review state.
		Cards []cardapi.Card `json:"cards"`
	}
	// Result describes a completed or canceled native archive operation.
	Result = struct {
		// Filename is the selected filename without its local directory.
		Filename string `json:"filename"`
		// FolderIDs contains newly created folder identifiers after import.
		FolderIDs []string `json:"folder_ids"`
		// FolderCount counts the transferred folders.
		FolderCount int `json:"folder_count"`
		// CardCount counts the transferred cards.
		CardCount int `json:"card_count"`
		// MediaCount counts the transferred media files.
		MediaCount int `json:"media_count"`
		// Canceled indicates that native selection was dismissed.
		Canceled bool `json:"canceled"`
	}
)
