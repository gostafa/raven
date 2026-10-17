// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"errors"

	libraryports "raven/internal/shared/desktopmedia/ports"
)

var (
	_ libraryports.Media       = (*Picker)(nil)
	_ libraryports.MediaRoutes = (*Registry)(nil)

	errInvalidMediaType                 = errors.New("invalid media type")
	errFileSelectionIsUnavailable       = errors.New("file selection is unavailable")
	errSelectANonemptyMediaFile         = errors.New("select a nonempty media file")
	errChooseASupportedFileForThisMedia = errors.New("choose a supported file for this media type")
)
