// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"raven/internal/features/reminders/ports"
)

var _ ports.Control = (*Worker)(nil)
