// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"raven/internal/features/folderarchive/ports"
)

var _ ports.Transfer = (*Service)(nil)
