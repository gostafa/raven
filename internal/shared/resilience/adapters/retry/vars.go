// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package retry

import (
	"raven/internal/shared/cache/ports"
)

var (
	_ ports.ReadRetry = (*Reads)(nil)
	_ ports.ReadRetry = Operation(nil)
)
