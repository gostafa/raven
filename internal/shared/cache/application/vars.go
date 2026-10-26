// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"errors"
	"io"
)

var (
	_              io.ReadSeeker = (*Reader)(nil)
	errInvalidSeek               = errors.New("invalid media seek")
)
