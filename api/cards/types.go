// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package cards defines Raven public cards data contracts.
package cards

type (
	// Card contains ordered content and its aggregate review times.
	Card struct {
		// Items contains ordered, runtime-validated content records.
		Items any `json:"items,omitempty"`
		// ID identifies this card.
		ID string `json:"id"`
		// FolderID identifies the owning folder.
		FolderID string `json:"folder_id"`
		// CreatedAt is the creation time in Unix milliseconds.
		CreatedAt int64 `json:"created_at"`
		// DueAt is the earliest item due time in Unix milliseconds.
		DueAt int64 `json:"due_at"`
		// LastReviewed is the latest manual review time in Unix milliseconds.
		LastReviewed int64 `json:"last_reviewed"`
	}

	// CreateInput identifies a folder and the content of a new card.
	CreateInput struct {
		// Items contains JSON-valued drafts validated before mutation.
		Items any `json:"items"`
		// FolderID identifies the owning folder.
		FolderID string `json:"folder_id"`
	}

	// UpdateInput identifies an existing card and its replacement content.
	UpdateInput struct {
		// Items contains JSON-valued replacement drafts validated before mutation.
		Items any `json:"items"`
		// FolderID identifies the owning folder.
		FolderID string `json:"folder_id"`
		// CardID identifies the card to update.
		CardID string `json:"card_id"`
	}

	// ReviewInput identifies an owned content item and its user rating.
	ReviewInput struct {
		// FolderID identifies the owning folder.
		FolderID string `json:"folder_id"`
		// CardID identifies the owning card.
		CardID string `json:"card_id"`
		// ItemID identifies the content to review.
		ItemID string `json:"item_id"`
		// Rating records the user's review grade from one through four.
		Rating int `json:"rating"`
	}
)
