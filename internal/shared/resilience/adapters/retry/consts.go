// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package retry

import (
	"time"
)

const (
	zeroAttempts     = 0
	singleAttempt    = 1
	readAttempts     = 3
	readTimeout      = 3 * time.Second
	readInitial      = 100 * time.Millisecond
	readMaximum      = time.Second
	initAttempts     = 5
	initTimeout      = 15 * time.Second
	initInitial      = 250 * time.Millisecond
	initMaximum      = 2 * time.Second
	readOperation    = "read"
	operationKey     = "operation"
	attemptKey       = "attempt"
	delayKey         = "delay"
	errorKey         = "error"
	operationMessage = "operation attempt completed"
)
