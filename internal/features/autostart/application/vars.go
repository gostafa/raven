// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"errors"
)

var errLoginStartupNotChanged = errors.New(
	"login startup was not changed; check your operating system's startup settings and any required approval",
)
