// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"errors"
	"sync"
	"testing"
)

type preferencesStub struct {
	value   string
	failure error
}

func (stub *preferencesStub) LoadLanguagePreference() (string, error) {
	return stub.value, stub.failure
}

func (stub *preferencesStub) SaveLanguagePreference(value string) error {
	if stub.failure != nil {
		return stub.failure
	}
	stub.value = value
	return nil
}

type systemStub struct {
	values  []string
	failure error
}

func (stub *systemStub) Locales() ([]string, error) { return stub.values, stub.failure }

func TestDurableLanguageChangesAndSystemRefresh(t *testing.T) {
	t.Parallel()
	preferences := &preferencesStub{value: "system", failure: nil}
	system := &systemStub{values: []string{"ar-EG"}}
	var warnings []error
	service := New(preferences, system, func(err error) { warnings = append(warnings, err) })
	service.Initialize()
	if service.Current().Locale != "ar" {
		t.Fatal(service.Current())
	}
	calls := 0
	cancel := service.Subscribe(func() {
		calls++
		if service.Current().Preference != preferences.value {
			t.Error("published before commit")
		}
	})
	state, err := service.Set("en")
	if err != nil || state.Revision != 2 || state.Direction != "ltr" {
		t.Fatal(state, err)
	}
	system.values = []string{"ar-SA"}
	service.Refresh()
	if calls != 1 {
		t.Fatal("explicit language changed on refresh")
	}
	if _, err = service.Set("fr"); !errors.Is(err, ErrInvalidPreference) {
		t.Fatal(err)
	}
	preferences.failure = errors.New("disk full")
	if _, err = service.Set("ar"); err == nil {
		t.Fatal("ignored failed write")
	}
	if service.Current() != state || calls != 1 {
		t.Fatal("failed write changed visible state")
	}
	if len(warnings) != 1 || !errors.Is(warnings[0], preferences.failure) {
		t.Fatal("failed write omitted diagnostic logging", warnings)
	}
	preferences.failure = nil
	if _, err = service.Set("system"); err != nil {
		t.Fatal(err)
	}
	system.values = []string{"en-US"}
	service.Refresh()
	if service.Current().Locale != "en" || calls != 3 {
		t.Fatal(service.Current(), calls)
	}
	cancel()
	if _, err = service.Set("ar"); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatal("listener retained after disposal")
	}
}

func TestInvalidAndUnavailablePreferencesFallBackWithoutOverwriting(t *testing.T) {
	t.Parallel()
	for _, preference := range []string{"removed", "system"} {
		preferences := &preferencesStub{value: preference, failure: errors.New("unavailable")}
		warnings := 0
		service := New(preferences, &systemStub{values: []string{"ar"}}, func(error) { warnings++ })
		service.Initialize()
		if service.Current().Locale != "ar" || preferences.value != preference || warnings != 1 {
			t.Fatal(service.Current(), preferences, warnings)
		}
	}
}

func TestConcurrentChangesAreSerialized(t *testing.T) {
	t.Parallel()
	service := New(
		&preferencesStub{value: "en", failure: nil},
		&systemStub{values: nil},
		func(err error) { t.Error(err) },
	)
	service.Initialize()
	var workers sync.WaitGroup
	for index := range 20 {
		workers.Go(func() {
			preference := "en"
			if index%2 == 0 {
				preference = "ar"
			}
			if _, err := service.Set(preference); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	if service.Current().Revision < 2 {
		t.Fatal(service.Current())
	}
}

func TestSystemLookupFailureFallsBackToEnglish(t *testing.T) {
	t.Parallel()
	warnings := 0
	service := New(
		&preferencesStub{value: "system"},
		&systemStub{failure: errors.New("OS unavailable")},
		func(error) { warnings++ },
	)
	service.Initialize()
	if service.Current().Locale != "en" || warnings != 1 {
		t.Fatal(service.Current(), warnings)
	}
}
