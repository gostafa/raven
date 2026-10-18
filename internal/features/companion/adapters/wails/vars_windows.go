//go:build windows && !server

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"errors"
)

var errWidgetProcedureReturnedZero = errors.New("native widget procedure returned zero")
