// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package fsrs

import (
	"encoding/json"
	"errors"
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
		execute: func(request *scheduleRequest) (state, decision json.RawMessage, failure error) {
			state, decision, err := executeSchedule(runtime, request)

			return state, decision, errors.Join(err)
		},
	}
}

// Initialize creates initial spaced repetition state and its scheduling decision.
func (scheduler Scheduler) Initialize(
	now time.Time,
) (state, decision json.RawMessage, failure error) {
	state, decision, err := scheduler.execute(
		&scheduleRequest{
			operation: initializeCommand,
			now:       now,
			value:     nil,
			card:      nil,
			rating:    unrated,
		},
	)

	return state, decision, errors.Join(err)
}

// Next advances JSON-valued scheduling state for the supplied rating.
func (scheduler Scheduler) Next(
	schedule any,
	now time.Time,
	rating int,
) (state, decision json.RawMessage, failure error) {
	state, decision, err := scheduler.execute(&scheduleRequest{
		operation: "next", value: schedule, now: now, rating: rating, card: nil,
	})

	return state, decision, errors.Join(err)
}

// Validate checks JSON-valued persisted scheduling state with FSRS.
func (scheduler Scheduler) Validate(schedule any) error {
	return errors.Join(
		schedulingFailure(
			scheduler.execute(
				&scheduleRequest{
					operation: validateCommand,
					value:     schedule,
					card:      nil,
					now:       time.Time{},
					rating:    unrated,
				},
			),
		),
	)
}

func executeSchedule(
	runtime *schedulerRuntime,
	request *scheduleRequest,
) (state, decision json.RawMessage, failure error) {
	if request.operation == initializeCommand {
		card := fsrs.NewCard(request.now)

		request.card, request.rating = &card, runtime.minimumRating

		encodedState, encodedDecision, err := encodedAdvance(runtime, request)

		return encodedState, encodedDecision, errors.Join(err)
	}

	state, decision, err := executeExisting(runtime, request)

	return state, decision, errors.Join(err)
}

func executeExisting(
	runtime *schedulerRuntime,
	request *scheduleRequest,
) (state, decision json.RawMessage, failure error) {
	schedule, err := contentapi.Decode[reviewapi.Schedule](request.value)
	if err != nil {
		return nil, nil, fmt.Errorf("decode scheduling state: %w", err)
	}

	if request.operation == validateCommand {
		return nil, nil, errors.Join(validateSchedule(runtime, &schedule))
	}

	card := toCard(&schedule)

	request.card = &card
	state, decision, err = encodedAdvance(runtime, request)

	return state, decision, errors.Join(err)
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
	request *scheduleRequest,
) (reviewapi.Schedule, reviewapi.Log, error) {
	if request.rating < runtime.minimumRating || request.rating > runtime.maximumRating {
		return reviewapi.Schedule{}, reviewapi.Log{}, errInvalidRating
	}

	nativeRating, err := ratingFor(request.rating)
	if err != nil {
		return reviewapi.Schedule{}, reviewapi.Log{}, fmt.Errorf("translate rating: %w", err)
	}

	result, err := runtime.next(*request.card, request.now, nativeRating)

	return fromCard(&result.Card), fromLog(&result.ReviewLog), errors.Join(err)
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
	request *scheduleRequest,
) (state, decision json.RawMessage, failure error) {
	schedule, log, err := advance(runtime, request)
	if err != nil {
		return nil, nil, fmt.Errorf("advance schedule: %w", err)
	}

	state, decision, err = encodeSchedule(&schedule, &log)

	return state, decision, errors.Join(err)
}

func encodeSchedule(
	schedule *reviewapi.Schedule,
	log *reviewapi.Log,
) (state, decision json.RawMessage, failure error) {
	state, err := json.Marshal(schedule)
	if err != nil {
		return nil, nil, fmt.Errorf("encode schedule: %w", err)
	}

	decision, err = json.Marshal(log)
	if err != nil {
		return nil, nil, fmt.Errorf("encode scheduling decision: %w", err)
	}

	return state, decision, nil
}

func schedulingFailure(_, _ json.RawMessage, err error) error { return errors.Join(err) }
