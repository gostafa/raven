// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package retry

import (
	"errors"

	"golang.org/x/sys/windows"
)

func platformTransient(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
