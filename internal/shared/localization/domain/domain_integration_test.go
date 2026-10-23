// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package domain_test

import (
	"testing"

	"raven/internal/shared/localization/domain"
)

func TestResolveLanguage(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		preference string
		locales    []string
		want       string
	}{
		{"system", []string{"fr-FR", "ar-EG", "en-US"}, "ar"},
		{"system", []string{"en-GB", "ar"}, "en"},
		{"system", []string{"", "_", "AR_SA.UTF-8"}, "ar"},
		{"system", nil, "en"},
		{"removed", []string{"ar"}, "ar"},
		{"en", []string{"ar"}, "en"},
		{"ar", []string{"en"}, "ar"},
	} {
		state := domain.Resolve(test.preference, test.locales)
		if state.Locale != test.want {
			t.Fatalf("%q %v: %+v", test.preference, test.locales, state)
		}
		if (state.Direction == "rtl") != (test.want == "ar") {
			t.Fatalf("wrong direction: %+v", state)
		}
	}
}
