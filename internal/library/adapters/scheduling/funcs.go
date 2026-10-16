// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package scheduling

import (
	"fmt"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"

	"raven/internal/library"
)

// New constructs the adapter with its required runtime dependencies.
func New() Scheduler { return Scheduler{engine: fsrs.NewFSRS(fsrs.DefaultParam())} }

// Initialize creates the initial spaced repetition schedule.
func (scheduler Scheduler) Initialize(now time.Time) (library.Schedule, library.ReviewLog, error) {
	card := fsrs.NewCard(now)

	result2, result3, forwardErr := scheduler.advance(&card, now, minimumRating)
	if forwardErr != nil {
		return result2, result3, fmt.Errorf("initialize: %w", forwardErr)
	}

	return result2, result3, nil
}

// Next advances a spaced repetition schedule for a user rating.
func (scheduler Scheduler) Next(
	card library.Schedule,
	now time.Time,
	rating int,
) (library.Schedule, library.ReviewLog, error) {
	native := toCard(&card)

	result4, result5, forwardErr2 := scheduler.advance(&native, now, rating)
	if forwardErr2 != nil {
		return result4, result5, fmt.Errorf("next: %w", forwardErr2)
	}

	return result4, result5, nil
}

// Validate checks that a persisted schedule is accepted by FSRS.
func (scheduler Scheduler) Validate(card library.Schedule) error {
	_, err := scheduler.engine.Repeat(toCard(&card), card.LastReview)
	if err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	return nil
}

func (scheduler Scheduler) advance(
	card *fsrs.Card,
	now time.Time,
	rating int,
) (library.Schedule, library.ReviewLog, error) {
	if rating < minimumRating || rating > maximumRating {
		return library.Schedule{}, library.ReviewLog{}, errInvalidRating
	}

	result, err := scheduler.engine.Next(*card, now, fsrs.Rating(rating))

	return fromCard(&result.Card), fromLog(&result.ReviewLog), err
}

func toCard(value *library.Schedule) fsrs.Card {
	return fsrs.Card{
		Due:            value.Due,
		Stability:      value.Stability,
		Difficulty:     value.Difficulty,
		ScheduledDays:  value.ScheduledDays,
		Reps:           value.Reps,
		Lapses:         value.Lapses,
		State:          fsrs.State(value.State),
		LastReview:     value.LastReview,
		RemainingSteps: value.RemainingSteps,
	}
}

func fromCard(value *fsrs.Card) library.Schedule {
	return library.Schedule{
		Due:            value.Due,
		Stability:      value.Stability,
		Difficulty:     value.Difficulty,
		ScheduledDays:  value.ScheduledDays,
		Reps:           value.Reps,
		Lapses:         value.Lapses,
		State:          int8(value.State),
		LastReview:     value.LastReview,
		RemainingSteps: value.RemainingSteps,
	}
}

func fromLog(value *fsrs.ReviewLog) library.ReviewLog {
	return library.ReviewLog{
		Rating:         int8(value.Rating),
		Due:            value.Due,
		ScheduledDays:  value.ScheduledDays,
		Review:         value.Review,
		State:          int8(value.State),
		Stability:      value.Stability,
		Difficulty:     value.Difficulty,
		RemainingSteps: value.RemainingSteps,
	}
}
