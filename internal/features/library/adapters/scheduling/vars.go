// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package scheduling

import (
	"errors"

	"raven/internal/features/library"
)

var (
	_                library.Scheduler = (*Scheduler)(nil)
	errInvalidRating                   = errors.New("invalid review rating")
)
