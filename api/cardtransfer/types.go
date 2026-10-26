// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package cardtransfer defines public card transfer contracts.
package cardtransfer

import (
	cardapi "raven/api/cards"
)

type (
	// Reference identifies a card within its owning folder.
	Reference struct {
		// FolderID identifies the owning folder.
		FolderID string `json:"folder_id"`
		// CardID identifies the card.
		CardID string `json:"card_id"`
	}
	// TransferInput requests an atomic batch of card copies or moves.
	TransferInput = struct {
		Operation           string      `json:"operation"`
		DestinationFolderID string      `json:"destination_folder_id"`
		Sources             []Reference `json:"sources"`
	}
	// TransferEntry maps one source to its destination card.
	TransferEntry = struct {
		// Source identifies the original card.
		Source Reference `json:"source"`
		// Card contains the resulting destination card.
		Card cardapi.Card `json:"card"`
	}
	// TransferResult contains transferred cards in source order.
	TransferResult = struct {
		// Entries maps original cards to their saved destination records.
		Entries []TransferEntry `json:"entries"`
	}
)
