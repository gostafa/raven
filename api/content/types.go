// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package content defines Raven public content data contracts.
package content

type (
	// Item contains one text or media item and its review history.
	Item struct {
		Schedule        any    `json:"schedule"`
		Reviews         any    `json:"reviews"`
		Kind            string `json:"kind"`
		FolderID        string `json:"folder_id"`
		Text            string `json:"text,omitempty"`
		CardID          string `json:"card_id"`
		MIMEType        string `json:"mime_type,omitempty"`
		RelativePath    string `json:"relative_path,omitempty"`
		ID              string `json:"id"`
		OriginalName    string `json:"original_name,omitempty"`
		LastNotifiedDue int64  `json:"last_notified_due"`
		Position        int    `json:"position"`
		ByteSize        int64  `json:"byte_size,omitempty"`
		Hint            bool   `json:"hint,omitempty"`
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
		ID         string `json:"id,omitempty"`
		Kind       string `json:"kind"`
		Text       string `json:"text,omitempty"`
		SourcePath string `json:"source_path,omitempty"`
		Hint       bool   `json:"hint,omitempty"`
	}
)
