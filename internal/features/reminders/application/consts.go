// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"time"
)

const (
	enableCommand = "enable"
	startCommand  = "start"
	wakeCommand   = "wake"
	stopCommand   = "stop"
	pollInterval  = 30 * time.Second
	wakeBuffer    = 1

	zero = 0
)
