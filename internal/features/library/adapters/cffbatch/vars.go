// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package cffbatch

import (
	"errors"

	"raven/internal/features/library/ports"
)

var (
	_               ports.BatchExecutor = (*Executor)(nil)
	errTaskPanicked                     = errors.New("batch task panicked")
)
