// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package fsrs

import (
	"errors"

	"raven/internal/shared/boltstore/ports"
)

var (
	_                      ports.Scheduler = (*Scheduler)(nil)
	errInvalidRating                       = errors.New("invalid review rating")
	errIncompleteSchedules                 = errors.New("incomplete review rating schedules")
)
