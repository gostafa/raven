// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package native

import (
	"embed"
	"errors"

	desktop "raven/internal/features/desktop/ports"
	reminders "raven/internal/features/reminders/ports"
	media "raven/internal/shared/desktopmedia/ports"
)

var (
	//go:embed *.json
	catalogs          embed.FS
	errMissingCatalog                      = errors.New("missing native catalog")
	_                 desktop.Localization = (*Messages)(nil)
	_                 reminders.Messages   = (*Messages)(nil)
	_                 media.Messages       = (*Messages)(nil)
)
