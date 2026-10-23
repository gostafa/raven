// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines localization's persistence and operating-system boundaries.
package ports

import (
	"raven/internal/shared/localization/domain"
)

type (
	// Preferences stores the language independently of library exports.
	Preferences interface {
		LoadLanguagePreference() (string, error)
		SaveLanguagePreference(preference string) error
	}
	// SystemLocales supplies the user's ordered operating-system language choices.
	SystemLocales interface{ Locales() ([]string, error) }
	// LocaleSource supplies current language and change notifications to native messages.
	LocaleSource interface {
		Current() domain.State
		Subscribe(listener func()) func()
		Refresh()
	}
)
