// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	autostart "raven/internal/features/autostart/application"
)

var _ autostart.Backend = (*Backend)(nil)
