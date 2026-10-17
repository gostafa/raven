// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package scheduling adapts FSRS scheduling to library contracts.
package scheduling

import (
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"

	library "raven/internal/features/library/domain"
)

type (
	// Scheduler translates between FSRS state and library schedules.
	Scheduler struct {
		engine schedulingEngine
		rules  *library.Rules
	}

	schedulingEngine interface {
		Next(card fsrs.Card, now time.Time, rating fsrs.Rating) (fsrs.SchedulingInfo, error)
		Repeat(card fsrs.Card, now time.Time) (fsrs.RecordLog, error)
	}
)
