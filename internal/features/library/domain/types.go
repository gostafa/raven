// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package domain defines library contracts and content workflows.
package domain

import (
	"time"

	contentapi "raven/api/content"
	reviewapi "raven/api/reviews"
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

	// Scheduler initializes, advances, and validates spaced repetition schedules.
	Scheduler interface {
		Initialize(now time.Time) (reviewapi.Schedule, reviewapi.Log, error)
		Next(
			schedule *reviewapi.Schedule,
			now time.Time,
			rating int,
		) (reviewapi.Schedule, reviewapi.Log, error)
		Validate(schedule *reviewapi.Schedule) error
	}
	// PreparationDependencies supplies media import and identifier generation.
	PreparationDependencies struct {
		ImportMedia func(*contentapi.Item, string) (contentapi.Item, string, error)
		NewID       func() (string, error)
	}

	itemPreparation struct {
		rules       *Rules
		previous    map[string]contentapi.Item
		seen        map[string]bool
		importMedia func(*contentapi.Item, string) (contentapi.Item, string, error)
		newID       func() (string, error)
		imported    []string
	}
)
