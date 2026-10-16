// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package content defines Raven public content data contracts.
package content

import (
	reviewapi "raven/api/reviews"
)

type (
	// Item contains one text or media item and its review history.
	Item struct {
		// Text contains plain text for a text item.
		Text string `json:"text,omitempty"`
		// OriginalName is the original media filename.
		OriginalName string `json:"original_name,omitempty"`
		// ID identifies this record.
		ID string `json:"id"`
		// FolderID identifies the owning folder.
		FolderID string `json:"folder_id"`
		// CardID identifies the owning card.
		CardID string `json:"card_id"`
		// Kind identifies text, image, audio, or video content.
		Kind string `json:"kind"`
		// MIMEType is the validated media content type.
		MIMEType string `json:"mime_type,omitempty"`
		// RelativePath is the media path relative to Raven storage.
		RelativePath string `json:"relative_path,omitempty"`
		// Reviews contains the item scheduling history.
		Reviews []Review `json:"reviews"`
		// Schedule contains the current item scheduling state.
		Schedule reviewapi.Schedule `json:"schedule"`
		// LastNotifiedDue is the last acknowledged due time in Unix milliseconds.
		LastNotifiedDue int64 `json:"last_notified_due"`
		// Position is the zero-based position within the card.
		Position int `json:"position"`
		// ByteSize is the media size in bytes.
		ByteSize int64 `json:"byte_size,omitempty"`
	}

	// Review records a scheduling decision and whether it was automatic.
	Review struct {
		// Log contains the scheduling result for this review.
		Log reviewapi.Log `json:"log"`
		// Automatic marks initialization rather than a user rating.
		Automatic bool `json:"automatic"`
	}

	// Draft describes new content or an edit to an existing item.
	Draft struct {
		// ID identifies this record.
		ID string `json:"id,omitempty"`
		// Kind identifies text, image, audio, or video content.
		Kind string `json:"kind"`
		// Text contains plain text for a text item.
		Text string `json:"text,omitempty"`
		// SourcePath is the selected local filesystem path.
		SourcePath string `json:"source_path,omitempty"`
	}
)
