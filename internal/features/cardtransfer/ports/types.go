// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines card transfer workflow and persistence boundaries.
package ports

type (
	// Library transfers a validated batch atomically with its owned media.
	Library interface {
		ApplyCardTransfer(input string) (string, error)
	}
	// Transfer exposes card transfers to the desktop facade.
	Transfer interface {
		TransferCards(input string) (string, error)
	}
)
