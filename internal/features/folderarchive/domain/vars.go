// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package domain

import (
	"errors"
)

var (
	// ErrInvalidArchive reports an unsupported or inconsistent archive.
	ErrInvalidArchive = errors.New("invalid Raven folder archive")
	// ErrInvalidMode requires an explicit supported import option.
	ErrInvalidMode = errors.New("choose start from zero or continue from last review")
)
