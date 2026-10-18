// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package reviews defines Raven public reviews data contracts.
package reviews

import (
	"time"
)

type (
	// Schedule contains the current spaced repetition state of a content item.
	Schedule struct {
		// Due is the next scheduled review time.
		Due time.Time `json:"due"`
		// LastReview is the previous scheduled review time.
		LastReview time.Time `json:"last_review"`
		// Stability is the estimated memory stability in days.
		Stability float64 `json:"stability"`
		// Difficulty is the estimated difficulty of the content.
		Difficulty float64 `json:"difficulty"`
		// ScheduledDays is the interval assigned by the scheduler in days.
		ScheduledDays uint64 `json:"scheduled_days"`
		// Reps counts completed scheduling steps.
		Reps uint64 `json:"reps"`
		// Lapses counts forgotten reviews.
		Lapses uint64 `json:"lapses"`
		// RemainingSteps counts the remaining learning steps.
		RemainingSteps int `json:"remaining_steps"`
		// State identifies the scheduler learning phase.
		State int8 `json:"state"`
	}

	// Log records the spaced repetition state at a review.
	Log struct {
		// Due is the next scheduled review time.
		Due time.Time `json:"due"`
		// Review is the time the review was recorded.
		Review time.Time `json:"review"`
		// ScheduledDays is the interval assigned by the scheduler in days.
		ScheduledDays uint64 `json:"scheduled_days"`
		// Stability is the estimated memory stability in days.
		Stability float64 `json:"stability"`
		// Difficulty is the estimated difficulty of the content.
		Difficulty float64 `json:"difficulty"`
		// RemainingSteps counts the remaining learning steps.
		RemainingSteps int `json:"remaining_steps"`
		// Rating is the user rating supplied to the scheduler.
		Rating int8 `json:"rating"`
		// State identifies the scheduler learning phase.
		State int8 `json:"state"`
	}

	// DueOccurrence identifies the precise item schedule acknowledged by a notification.
	DueOccurrence struct {
		// FolderID identifies the owning folder.
		FolderID string `json:"FolderID"`
		// CardID identifies the owning card.
		CardID string `json:"CardID"`
		// ItemID identifies the content item.
		ItemID string `json:"ItemID"`
		// Due is the next scheduled review time.
		Due int64 `json:"Due"`
	}

	// Attempt contains the time and rating of a manual review.
	Attempt struct {
		At     time.Time
		Rating int
	}
)
