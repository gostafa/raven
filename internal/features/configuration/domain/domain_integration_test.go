package domain_test

import (
	"encoding/json"
	"reflect"
	"testing"

	companion "raven/internal/features/companion/domain"
	"raven/internal/features/configuration/adapters/yaml"
	"raven/internal/features/configuration/domain"
	library "raven/internal/shared/boltstore/domain"
)

func TestValidationRejectsMissingAndIncompatibleContracts(t *testing.T) {
	for _, name := range []string{
		"ApplicationID", "LogErrorKey", "LogOperationKey", "PreferencesStorageKey",
		"SchedulesChanged", "ShowReviews", "OpenFloatingReview", "CloseFloatingReview",
	} {
		t.Run(name, func(t *testing.T) {
			settings := newSettings(t)
			settings.strings[name] = ""
			if err := domain.Validate(settings); err == nil {
				t.Fatal("accepted missing value")
			}
		})
	}
	for name, change := range map[string]func(*testSettings){
		"text kind":      func(s *testSettings) { s.rules.TextKind = "" },
		"jpeg":           func(s *testSettings) { s.rules.MIMEJPEG = "" },
		"ogg":            func(s *testSettings) { s.rules.MIMEOggAudio = "" },
		"mp4":            func(s *testSettings) { s.rules.MIMEMP4Video = "" },
		"rating minimum": func(s *testSettings) { s.rules.MinimumRating = 0 },
		"rating maximum": func(s *testSettings) { s.rules.MaximumRating = 5 },
		"invalid route":  func(s *testSettings) { s.strings["MediaRoutePrefix"] = ":invalid" },
		"relative route": func(s *testSettings) { s.strings["MediaRoutePrefix"] = "media/" },
		"missing route":  func(s *testSettings) { s.strings["PreviewRoutePrefix"] = "" },
		"query route":    func(s *testSettings) { s.strings["PreviewRoutePrefix"] = "/preview/?x=/" },
		"overlap":        func(s *testSettings) { s.strings["PreviewRoutePrefix"] = s.MediaRoutePrefix() },
		"nested route":   func(s *testSettings) { s.strings["PreviewRoutePrefix"] = s.MediaRoutePrefix() + "preview/" },
		"events":         func(s *testSettings) { s.strings["ShowReviews"] = s.SchedulesChanged() },
	} {
		t.Run(name, func(t *testing.T) {
			settings := newSettings(t)
			change(settings)
			if err := domain.Validate(settings); err == nil {
				t.Fatal("accepted invalid contract")
			}
		})
	}
}

func TestGeometryAndDefaultValidation(t *testing.T) {
	for _, name := range []string{"WidgetSize", "ReviewWidth", "ReviewHeight", "ReviewGap"} {
		settings := newSettings(t)
		reflect.ValueOf(&settings.layout).Elem().FieldByName(name).SetInt(0)
		if err := domain.Validate(settings); err == nil {
			t.Fatal("accepted missing geometry")
		}
	}
	settings := newSettings(t)
	if err := domain.Validate(settings); err != nil {
		t.Fatal(err)
	}
}

type testSettings struct {
	domain.Settings
	strings map[string]string
	rules   library.Rules
	layout  companion.Layout
}

func newSettings(t *testing.T) *testSettings {
	t.Helper()
	snapshot, err := yaml.Load()
	if err != nil {
		t.Fatal(err)
	}
	return &testSettings{
		Settings: snapshot,
		strings:  make(map[string]string),
		rules:    domain.Library(snapshot),
		layout:   domain.Layout(snapshot),
	}
}

func (settings *testSettings) Library() json.RawMessage {
	value, err := json.Marshal(settings.rules)
	if err != nil {
		panic(err)
	}
	return value
}

func (settings *testSettings) Layout() json.RawMessage {
	value, err := json.Marshal(settings.layout)
	if err != nil {
		panic(err)
	}
	return value
}

func (settings *testSettings) ApplicationID() string {
	if value, ok := settings.strings["ApplicationID"]; ok {
		return value
	}
	return settings.Settings.ApplicationID()
}

func (settings *testSettings) LogErrorKey() string {
	if value, ok := settings.strings["LogErrorKey"]; ok {
		return value
	}
	return settings.Settings.LogErrorKey()
}

func (settings *testSettings) LogOperationKey() string {
	if value, ok := settings.strings["LogOperationKey"]; ok {
		return value
	}
	return settings.Settings.LogOperationKey()
}

func (settings *testSettings) PreferencesStorageKey() string {
	if value, ok := settings.strings["PreferencesStorageKey"]; ok {
		return value
	}
	return settings.Settings.PreferencesStorageKey()
}

func (settings *testSettings) SchedulesChanged() string {
	if value, ok := settings.strings["SchedulesChanged"]; ok {
		return value
	}
	return settings.Settings.SchedulesChanged()
}

func (settings *testSettings) ShowReviews() string {
	if value, ok := settings.strings["ShowReviews"]; ok {
		return value
	}
	return settings.Settings.ShowReviews()
}

func (settings *testSettings) OpenFloatingReview() string {
	if value, ok := settings.strings["OpenFloatingReview"]; ok {
		return value
	}
	return settings.Settings.OpenFloatingReview()
}

func (settings *testSettings) CloseFloatingReview() string {
	if value, ok := settings.strings["CloseFloatingReview"]; ok {
		return value
	}
	return settings.Settings.CloseFloatingReview()
}

func (settings *testSettings) MediaRoutePrefix() string {
	if value, ok := settings.strings["MediaRoutePrefix"]; ok {
		return value
	}
	return settings.Settings.MediaRoutePrefix()
}

func (settings *testSettings) PreviewRoutePrefix() string {
	if value, ok := settings.strings["PreviewRoutePrefix"]; ok {
		return value
	}
	return settings.Settings.PreviewRoutePrefix()
}

func TestUninitializedSettingsCannotProduceValidatedContracts(t *testing.T) {
	settings := &yaml.Settings{}
	if domain.Library(settings) != (library.Rules{}) ||
		domain.Layout(settings) != (companion.Layout{}) {
		t.Fatal("uninitialized configuration produced typed contracts")
	}
	if err := domain.Validate(settings); err == nil {
		t.Fatal("accepted uninitialized configuration")
	}
}
