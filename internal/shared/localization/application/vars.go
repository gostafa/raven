// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"errors"

	"raven/internal/shared/localization/ports"
)

var (
	// ErrInvalidPreference identifies an unsupported public language choice.
	ErrInvalidPreference                    = errors.New("unsupported language preference")
	_                    ports.LocaleSource = (*Service)(nil)
)
