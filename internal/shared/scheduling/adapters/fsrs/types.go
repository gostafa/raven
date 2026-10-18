// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package fsrs adapts FSRS scheduling to library contracts.
package fsrs

import (
	"encoding/json"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"
)

type (
	// Scheduler translates JSON-valued schedules through the consumer-owned scheduling port.
	Scheduler struct {
		execute func(*scheduleRequest) (json.RawMessage, json.RawMessage, error)
	}

	scheduleRequest struct {
		value     any
		card      *fsrs.Card
		now       time.Time
		operation string
		rating    int
	}

	schedulerRuntime struct {
		next          func(fsrs.Card, time.Time, fsrs.Rating) (fsrs.SchedulingInfo, error)
		repeat        func(fsrs.Card, time.Time) (fsrs.RecordLog, error)
		minimumRating int
		maximumRating int
	}

	schedulingEngine interface {
		Next(card fsrs.Card, now time.Time, rating fsrs.Rating) (fsrs.SchedulingInfo, error)
		Repeat(card fsrs.Card, now time.Time) (fsrs.RecordLog, error)
	}
)
