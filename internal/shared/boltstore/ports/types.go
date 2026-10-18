// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines persistence and scheduling boundaries.
package ports

import (
	"context"
	"encoding/json"
	"time"
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
		ListFolders() (string, error)
		CreateFolder(input string) (string, error)
		DeleteFolder(folderID string) error
	}

	// Cards persists card content and schedules.
	Cards interface {
		ListCards() (string, error)
		CreateCard(input string) (string, error)
		UpdateCard(input string) (string, error)
		DeleteCard(folderID, cardID string) error
		ReviewItem(input string) (string, error)
	}

	// ReviewSettings persists reminder acknowledgments and one-time initialization.
	ReviewSettings interface {
		MarkNotified(occurrences string) error
		ClaimNotificationPrompt() (bool, error)
		ClaimAutostartInitialization() (bool, error)
	}

	// Scheduler initializes, advances, and validates spaced repetition schedules.
	Scheduler interface {
		Initialize(now time.Time) (json.RawMessage, json.RawMessage, error)
		Next(
			schedule any,
			now time.Time,
			rating int,
		) (json.RawMessage, json.RawMessage, error)
		Validate(schedule any) error
	}
)
