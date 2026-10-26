// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package notifications

import (
	"raven/internal/features/desktop/ports"
)

var _ ports.Notifications = (*Notifier)(nil)
