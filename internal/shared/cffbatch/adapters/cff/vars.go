// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package cff

import (
	"errors"

	"raven/internal/shared/boltstore/ports"
	mediaports "raven/internal/shared/desktopmedia/ports"
)

var (
	_               mediaports.BatchExecutor = (*Executor)(nil)
	_               ports.BatchExecutor      = (*Executor)(nil)
	errTaskPanicked                          = errors.New("batch task panicked")
)
