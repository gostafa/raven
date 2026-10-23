// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package folders defines Raven public folders data contracts.
package folders

type (
	// ExportInput selects complete folders for a portable archive.
	ExportInput struct {
		// FolderIDs identifies the folders to export regardless of display filters.
		FolderIDs []string `json:"folder_ids"`
	}
	// Folder contains the metadata of a library folder.
	Folder struct {
		// ID identifies this record.
		ID string `json:"id"`
		// Title is the display title.
		Title string `json:"title"`
		// Description is the optional folder description.
		Description string `json:"description"`
		// Tags contains the normalized folder tags.
		Tags []string `json:"tags"`
	}

	// CreateInput contains the editable metadata for a new folder.
	CreateInput struct {
		// Title is the display title.
		Title string `json:"title"`
		// Description is the optional folder description.
		Description string `json:"description"`
		// Tags contains the normalized folder tags.
		Tags []string `json:"tags"`
	}
	// UpdateInput identifies a folder and its replacement metadata using the folder record contract.
	UpdateInput = Folder
)
