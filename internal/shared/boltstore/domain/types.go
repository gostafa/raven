// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package domain defines library contracts and content workflows.
package domain

import (
	"context"
	"encoding/json"
	"time"
)

type (
	// Rules supplies the shared content identifiers and supported rating range.
	Rules struct {
		// TextKind identifies plain text content.
		TextKind string `json:"text_kind" yaml:"text_kind"`
		// MIMEJPEG is the canonical content type for JPEG images.
		MIMEJPEG string `json:"mime_jpeg" yaml:"mime_jpeg"`
		// MIMEOggAudio is the canonical content type for Ogg audio.
		MIMEOggAudio string `json:"mime_ogg_audio" yaml:"mime_ogg_audio"`
		// MIMEMP4Video is the canonical content type for MP4 video.
		MIMEMP4Video string `json:"mime_mp4_video" yaml:"mime_mp4_video"`
		// MinimumRating is the lowest supported review grade.
		MinimumRating int `json:"minimum_rating" yaml:"minimum_rating"`
		// MaximumRating is the highest supported review grade.
		MaximumRating int `json:"maximum_rating" yaml:"maximum_rating"`
	}

	// PreparationInput supplies draft content and its previous owned records.
	PreparationInput struct {
		// Drafts contains JSON-valued editable content.
		Drafts any
		// Previous contains JSON-valued records owned by this card.
		Previous any
	}
	// PreparationDependencies supplies media import and identifier generation.
	PreparationDependencies struct {
		// Rules contains independently validated content policy.
		Rules any
		// Batch runs independent media imports to completion.
		Batch func(context.Context, []func(context.Context) error) error
		// ImportMedia copies media for a JSON-valued content record.
		ImportMedia func(json.RawMessage, string) (json.RawMessage, string, error)
		// NewID allocates a unique content identifier.
		NewID func() (string, error)
	}

	// ReviewDependencies supplies content policy and the scheduling capability.
	ReviewDependencies struct {
		// Rules contains independently validated rating policy.
		Rules any
		// Next advances JSON-valued scheduling state after a review.
		Next func(any, time.Time, int) (json.RawMessage, json.RawMessage, error)
	}

	mediaPreparation struct {
		rules       any
		items       any
		drafts      any
		batch       func(context.Context, []func(context.Context) error) error
		importMedia func(json.RawMessage, string) (json.RawMessage, string, error)
		textKind    string
		paths       []string
	}
	itemPreparation struct {
		previous any
		seen     map[string]bool
		newID    func() (string, error)
	}
)
