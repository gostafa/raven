// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines persistence and scheduling boundaries.
package ports

import (
	"context"
	"time"

	cardapi "raven/api/cards"
	folderapi "raven/api/folders"
	reviewapi "raven/api/reviews"
)

type (
	// BatchExecutor runs independent jobs with bounded concurrency. It returns the
	// first error in input order and drains started jobs even after cancellation.
	// Job failures do not skip other jobs; cancellation skips pending jobs.
	// Panics become errors, and an empty batch succeeds even when canceled.
	BatchExecutor interface {
		Run(ctx context.Context, jobs []func(context.Context) error) error
	}

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
)
