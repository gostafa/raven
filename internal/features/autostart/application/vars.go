// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"errors"

	desktopports "raven/internal/features/desktop/ports"
)

var (
	errLoginStartupNotChanged = errors.New(
		"login startup was not changed; check your operating system's startup settings and any required approval",
	)

	_ desktopports.Startup          = (*Controller)(nil)
	_ desktopports.LaunchVisibility = (*Visibility)(nil)
	_ desktopports.OpenRequests     = (*Requests)(nil)
)
