// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package media defines Raven public media data contracts.
package media

type (
	// Selection describes a selected file and its temporary preview URL.
	Selection struct {
		// SourcePath is the selected local filesystem path.
		SourcePath string `json:"source_path"`
		// OriginalName is the original media filename.
		OriginalName string `json:"original_name"`
		// PreviewURL is the temporary URL used to preview a selected file.
		PreviewURL string `json:"preview_url"`
		// ByteSize is the media size in bytes.
		ByteSize int64 `json:"byte_size"`
	}
)
