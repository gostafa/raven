// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package scheduling

import (
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"

	"raven/internal/library"
)

func New() library.Scheduler { return scheduler{} }
func (scheduler) Initialize(now time.Time) (library.Schedule, library.ReviewLog, error) {
	return advance(fsrs.NewCard(now), now, 1)
}

func (scheduler) Next(
	card library.Schedule,
	now time.Time,
	rating int,
) (library.Schedule, library.ReviewLog, error) {
	return advance(toCard(card), now, rating)
}

func (scheduler) Validate(card library.Schedule) error {
	_, err := fsrs.NewFSRS(fsrs.DefaultParam()).Repeat(toCard(card), card.LastReview)

	return err
}

func advance(
	card fsrs.Card,
	now time.Time,
	rating int,
) (library.Schedule, library.ReviewLog, error) {
	result, err := fsrs.NewFSRS(fsrs.DefaultParam()).Next(card, now, fsrs.Rating(rating))

	return fromCard(result.Card), fromLog(result.ReviewLog), err
}

func toCard(v library.Schedule) fsrs.Card {
	return fsrs.Card{
		Due:            v.Due,
		Stability:      v.Stability,
		Difficulty:     v.Difficulty,
		ScheduledDays:  v.ScheduledDays,
		Reps:           v.Reps,
		Lapses:         v.Lapses,
		State:          fsrs.State(v.State),
		LastReview:     v.LastReview,
		RemainingSteps: v.RemainingSteps,
	}
}

func fromCard(v fsrs.Card) library.Schedule {
	return library.Schedule{
		Due:            v.Due,
		Stability:      v.Stability,
		Difficulty:     v.Difficulty,
		ScheduledDays:  v.ScheduledDays,
		Reps:           v.Reps,
		Lapses:         v.Lapses,
		State:          int8(v.State),
		LastReview:     v.LastReview,
		RemainingSteps: v.RemainingSteps,
	}
}

func fromLog(v fsrs.ReviewLog) library.ReviewLog {
	return library.ReviewLog{
		Rating:         int8(v.Rating),
		Due:            v.Due,
		ScheduledDays:  v.ScheduledDays,
		Review:         v.Review,
		State:          int8(v.State),
		Stability:      v.Stability,
		Difficulty:     v.Difficulty,
		RemainingSteps: v.RemainingSteps,
	}
}
