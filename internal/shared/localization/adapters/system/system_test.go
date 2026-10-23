// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package system

import (
	"errors"
	"reflect"
	"testing"
)

func TestSystemReaderAndFailures(t *testing.T) {
	t.Parallel()
	if New().read == nil {
		t.Fatal("missing system reader")
	}
	failure := errors.New("system unavailable")
	for _, err := range []error{nil, failure} {
		reader := Locales{read: func() ([]string, error) { return []string{"ar-EG"}, err }}
		values, got := reader.Locales()
		if err != nil {
			if !errors.Is(got, failure) {
				t.Fatal(got)
			}
		} else if got != nil || !reflect.DeepEqual(values, []string{"ar-EG"}) {
			t.Fatal(values, got)
		}
	}
}
