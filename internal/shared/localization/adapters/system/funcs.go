// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package system

import (
	"fmt"

	locale "github.com/jeandeaual/go-locale"
)

// New allocates an OS language reader without querying the operating system.
func New() Locales { return Locales{read: locale.GetLocales} }

// Locales returns the ordered BCP 47 language preferences.
func (locales Locales) Locales() ([]string, error) {
	values, err := locales.read()
	if err != nil {
		return nil, fmt.Errorf("read system languages: %w", err)
	}

	return values, nil
}
