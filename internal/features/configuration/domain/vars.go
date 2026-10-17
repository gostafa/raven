// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package domain

import (
	"errors"
)

var (
	errInvalidConfiguration = errors.New("invalid application configuration")
	errOverlappingRoutes    = errors.New("configuration media routes overlap")
	errIncompatibleRatings  = errors.New(
		"configuration ratings must match FSRS ratings 1 through 4",
	)
)
