// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package fsrs

import (
	"fmt"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"

	reviewapi "raven/api/reviews"
	library "raven/internal/shared/boltstore/domain"
)

// New constructs the adapter with its required runtime dependencies.
func New(rules *library.Rules) Scheduler {
	settings := *rules

	return Scheduler{rules: &settings, engine: fsrs.NewFSRS(fsrs.DefaultParam())}
}

// Initialize creates the initial spaced repetition schedule.
func (scheduler Scheduler) Initialize(now time.Time) (reviewapi.Schedule, reviewapi.Log, error) {
	card := fsrs.NewCard(now)

	result2, result3, forwardErr := scheduler.advance(&card, now, scheduler.rules.MinimumRating)
	if forwardErr != nil {
		return result2, result3, fmt.Errorf("initialize: %w", forwardErr)
	}

	return result2, result3, nil
}

// Next advances a spaced repetition schedule for a user rating.
func (scheduler Scheduler) Next(
	card *reviewapi.Schedule,
	now time.Time,
	rating int,
) (reviewapi.Schedule, reviewapi.Log, error) {
	native := toCard(card)

	result4, result5, forwardErr2 := scheduler.advance(&native, now, rating)
	if forwardErr2 != nil {
		return result4, result5, fmt.Errorf("next: %w", forwardErr2)
	}

	return result4, result5, nil
}

// Validate checks that a persisted schedule is accepted by FSRS.
func (scheduler Scheduler) Validate(card *reviewapi.Schedule) error {
	records, err := scheduler.engine.Repeat(toCard(card), card.LastReview)
	if err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	if len(records) != scheduler.rules.MaximumRating {
		return errIncompleteSchedules
	}

	return nil
}

func (scheduler Scheduler) advance(
	card *fsrs.Card,
	now time.Time,
	rating int,
) (reviewapi.Schedule, reviewapi.Log, error) {
	if rating < scheduler.rules.MinimumRating || rating > scheduler.rules.MaximumRating {
		return reviewapi.Schedule{}, reviewapi.Log{}, errInvalidRating
	}

	nativeRating, err := ratingFor(rating)
	if err != nil {
		return reviewapi.Schedule{}, reviewapi.Log{}, fmt.Errorf("translate rating: %w", err)
	}

	result, err := scheduler.engine.Next(*card, now, nativeRating)

	return fromCard(&result.Card), fromLog(&result.ReviewLog), err
}

func toCard(value *reviewapi.Schedule) fsrs.Card {
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

func fromCard(value *fsrs.Card) reviewapi.Schedule {
	return reviewapi.Schedule{
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

func fromLog(value *fsrs.ReviewLog) reviewapi.Log {
	return reviewapi.Log{
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

func ratingFor(rating int) (fsrs.Rating, error) {
	if rating < int(fsrs.Again) || rating > int(fsrs.Easy) {
		return fsrs.Again, errInvalidRating
	}

	return fsrs.Rating(rating), nil
}
