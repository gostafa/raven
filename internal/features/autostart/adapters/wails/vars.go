// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	autostartports "raven/internal/features/autostart/ports"
)

var (
	_ autostartports.LoginDetection = Detector{}

	_ autostartports.Backend = (*Backend)(nil)
)
