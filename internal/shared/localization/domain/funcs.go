// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package domain

import (
	"strings"
)

// ValidPreference accepts only the choices offered by Raven.
func ValidPreference(value string) bool { return value == System || supported(value) }

func supported(value string) bool { return value == English || value == Arabic }

// Resolve chooses the first supported system language, retaining English fallback.
func Resolve(preference string, locales []string) State {
	if !ValidPreference(preference) {
		preference = System
	}

	locale := preference
	if preference == System {
		locale = systemLanguage(locales)
	}

	return State{
		Preference: preference,
		Locale:     locale,
		Direction:  direction(locale),
		Revision:   uninitialized,
	}
}

func direction(locale string) string {
	if locale == Arabic {
		return "rtl"
	}

	return "ltr"
}

func systemLanguage(locales []string) string {
	for index := range locales {
		base := baseLanguage(locales[index])
		if supported(base) {
			return base
		}
	}

	return English
}

func baseLanguage(tag string) string {
	normalized := strings.ReplaceAll(tag, "_", languageSeparator)
	end := strings.IndexAny(normalized, "-.")

	if end < uninitialized {
		return strings.ToLower(normalized)
	}

	return strings.ToLower(normalized[:end])
}
