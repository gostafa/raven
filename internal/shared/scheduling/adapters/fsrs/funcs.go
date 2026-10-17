// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package fsrs

import (
	"encoding/json"
	"fmt"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"

	contentapi "raven/api/content"
	reviewapi "raven/api/reviews"
	library "raven/internal/shared/boltstore/domain"
)

// New binds FSRS and immutable rating limits to the scheduling port.
func New(rules *library.Rules) Scheduler {
	return newScheduler(fsrs.NewFSRS(fsrs.DefaultParam()), rules)
}

func newScheduler(engine schedulingEngine, rules *library.Rules) Scheduler {
	runtime := &schedulerRuntime{
		next: engine.Next, repeat: engine.Repeat,
		minimumRating: rules.MinimumRating, maximumRating: rules.MaximumRating,
	}
	return Scheduler{
		execute: func(operation string, value any, now time.Time, rating int) (json.RawMessage, json.RawMessage, error) {
			return executeSchedule(runtime, operation, value, now, rating)
		},
	}
}

// Initialize creates the initial spaced repetition state and scheduling decision.
func (scheduler Scheduler) Initialize(now time.Time) (json.RawMessage, json.RawMessage, error) {
	return scheduler.execute("initialize", nil, now, 0)
}

// Next advances JSON-valued scheduling state for the supplied user rating.
func (scheduler Scheduler) Next(
	schedule any,
	now time.Time,
	rating int,
) (json.RawMessage, json.RawMessage, error) {
	return scheduler.execute("next", schedule, now, rating)
}

// Validate validates JSON-valued persisted scheduling state with FSRS.
func (scheduler Scheduler) Validate(schedule any) error {
	_, _, err := scheduler.execute("validate", schedule, time.Time{}, 0)
	return err
}

func executeSchedule(
	runtime *schedulerRuntime,
	operation string,
	value any,
	now time.Time,
	rating int,
) (json.RawMessage, json.RawMessage, error) {
	if operation == "initialize" {
		card := fsrs.NewCard(now)
		return encodedAdvance(runtime, &card, now, runtime.minimumRating)
	}
	return executeExisting(runtime, operation, value, now, rating)
}

func executeExisting(
	runtime *schedulerRuntime,
	operation string,
	value any,
	now time.Time,
	rating int,
) (json.RawMessage, json.RawMessage, error) {
	schedule, err := contentapi.Decode[reviewapi.Schedule](value)
	if err != nil {
		return nil, nil, fmt.Errorf("decode scheduling state: %w", err)
	}
	if operation == "validate" {
		return nil, nil, validateSchedule(runtime, &schedule)
	}
	card := toCard(&schedule)
	return encodedAdvance(runtime, &card, now, rating)
}

func validateSchedule(runtime *schedulerRuntime, schedule *reviewapi.Schedule) error {
	records, err := runtime.repeat(toCard(schedule), schedule.LastReview)
	if err != nil {
		return fmt.Errorf("validate schedule: %w", err)
	}
	if len(records) != runtime.maximumRating {
		return errIncompleteSchedules
	}
	return nil
}

func advance(
	runtime *schedulerRuntime,
	card *fsrs.Card,
	now time.Time,
	rating int,
) (reviewapi.Schedule, reviewapi.Log, error) {
	if rating < runtime.minimumRating || rating > runtime.maximumRating {
		return reviewapi.Schedule{}, reviewapi.Log{}, errInvalidRating
	}
	nativeRating, err := ratingFor(rating)
	if err != nil {
		return reviewapi.Schedule{}, reviewapi.Log{}, fmt.Errorf("translate rating: %w", err)
	}
	result, err := runtime.next(*card, now, nativeRating)
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

func encodedAdvance(
	runtime *schedulerRuntime,
	card *fsrs.Card,
	now time.Time,
	rating int,
) (json.RawMessage, json.RawMessage, error) {
	schedule, log, err := advance(runtime, card, now, rating)
	if err != nil {
		return nil, nil, fmt.Errorf("advance schedule: %w", err)
	}
	return encodeSchedule(&schedule, &log)
}

func encodeSchedule(
	schedule *reviewapi.Schedule,
	log *reviewapi.Log,
) (json.RawMessage, json.RawMessage, error) {
	state, err := json.Marshal(schedule)
	if err != nil {
		return nil, nil, fmt.Errorf("encode schedule: %w", err)
	}
	decision, err := json.Marshal(log)
	if err != nil {
		return nil, nil, fmt.Errorf("encode scheduling decision: %w", err)
	}
	return state, decision, nil
}
