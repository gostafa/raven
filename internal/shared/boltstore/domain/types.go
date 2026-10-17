// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package domain defines library contracts and content workflows.
package domain

import (
	contentapi "raven/api/content"
	"raven/internal/shared/boltstore/ports"
)

type (
	// Rules supplies the shared content identifiers and supported rating range.
	Rules struct {
		TextKind      string `yaml:"text_kind"`
		MIMEJPEG      string `yaml:"mime_jpeg"`
		MIMEOggAudio  string `yaml:"mime_ogg_audio"`
		MIMEMP4Video  string `yaml:"mime_mp4_video"`
		MinimumRating int    `yaml:"minimum_rating"`
		MaximumRating int    `yaml:"maximum_rating"`
	}

	// PreparationInput supplies draft content and its previous owned records.
	PreparationInput struct {
		Drafts   []contentapi.Draft
		Previous []contentapi.Item
	}
	// PreparationDependencies supplies media import and identifier generation.
	PreparationDependencies struct {
		Batch       ports.BatchExecutor
		ImportMedia func(*contentapi.Item, string) (contentapi.Item, string, error)
		NewID       func() (string, error)
	}

	mediaPreparation struct {
		dependencies *PreparationDependencies
		textKind     string
		items        []contentapi.Item
		drafts       []contentapi.Draft
		paths        []string
	}
	itemPreparation struct {
		previous map[string]contentapi.Item
		seen     map[string]bool
		newID    func() (string, error)
	}
)
