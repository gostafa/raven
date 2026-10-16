// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines library persistence and media boundaries.
package ports

import (
	"net/http"

	cardapi "raven/api/cards"
	folderapi "raven/api/folders"
	mediaapi "raven/api/media"
	reviewapi "raven/api/reviews"
)

type (
	// Repository provides library persistence and durable application settings.
	Repository interface {
		Folders
		Cards
		ReviewSettings
		Close() error
	}

	// Folders persists folder metadata and ownership.
	Folders interface {
		ListFolders() ([]folderapi.Folder, error)
		CreateFolder(input *folderapi.CreateInput) (folderapi.Folder, error)
		DeleteFolder(folderID string) error
	}

	// Cards persists card content and schedules.
	Cards interface {
		ListCards() ([]cardapi.Card, error)
		CreateCard(input *cardapi.CreateInput) (cardapi.Card, error)
		UpdateCard(input *cardapi.UpdateInput) (cardapi.Card, error)
		DeleteCard(folderID, cardID string) error
		ReviewItem(input *cardapi.ReviewInput) (cardapi.Card, error)
	}

	// ReviewSettings persists reminder acknowledgments and one-time initialization.
	ReviewSettings interface {
		MarkNotified(occurrences []reviewapi.DueOccurrence) error
		ClaimNotificationPrompt() (bool, error)
		ClaimAutostartInitialization() (bool, error)
	}

	// Media selects local files and serves temporary or saved media.
	Media interface {
		Select(kind string) ([]mediaapi.Selection, error)
		Middleware(next http.Handler) http.HandlerFunc
	}
)
