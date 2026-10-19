// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package content defines Raven public content data contracts.
package content

type (
	// Item contains one text or media item and its review history.
	Item struct {
		// Schedule contains the current review scheduling state.
		Schedule any `json:"schedule"`
		// Reviews contains the ordered review history.
		Reviews any `json:"reviews"`
		// Kind identifies text or the media category.
		Kind string `json:"kind"`
		// FolderID identifies the owning folder.
		FolderID string `json:"folder_id"`
		// Text contains the text content when Kind is text.
		Text string `json:"text,omitempty"`
		// CardID identifies the owning card.
		CardID string `json:"card_id"`
		// MIMEType identifies the stored media format.
		MIMEType string `json:"mime_type,omitempty"`
		// RelativePath locates media relative to library storage.
		RelativePath string `json:"relative_path,omitempty"`
		// ID identifies this content item.
		ID string `json:"id"`
		// OriginalName preserves the imported media filename.
		OriginalName string `json:"original_name,omitempty"`
		// LastNotifiedDue records the last acknowledged due time in Unix milliseconds.
		LastNotifiedDue int64 `json:"last_notified_due"`
		// Position is the item's zero-based order within its card.
		Position int `json:"position"`
		// ByteSize is the stored media size in bytes.
		ByteSize int64 `json:"byte_size,omitempty"`
		// Hint excludes this item from scheduled reviews.
		Hint bool `json:"hint,omitempty"`
	}

	// Review records a scheduling decision and whether it was automatic.
	Review struct {
		// Log contains the scheduling result for this review.
		Log any `json:"log"`
		// Automatic marks initialization rather than a user rating.
		Automatic bool `json:"automatic"`
	}

	// Draft describes new content or an edit to an existing item.
	Draft struct {
		// ID identifies retained content; an empty value creates a new item.
		ID string `json:"id,omitempty"`
		// Kind identifies text or the media category.
		Kind string `json:"kind"`
		// Text contains replacement text content.
		Text string `json:"text,omitempty"`
		// SourcePath identifies a local media file to import.
		SourcePath string `json:"source_path,omitempty"`
		// Hint excludes this item from scheduled reviews.
		Hint bool `json:"hint,omitempty"`
	}
)
