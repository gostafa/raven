package domain_test

import (
	"reflect"
	"testing"

	"raven/internal/features/configuration/adapters/yaml"
	"raven/internal/features/configuration/domain"
)

func TestValidationRejectsMissingAndIncompatibleContracts(t *testing.T) {
	for _, name := range []string{
		"ApplicationID", "LogErrorKey", "LogOperationKey", "PreferencesStorageKey",
		"SchedulesChanged", "ShowReviews", "OpenFloatingReview", "CloseFloatingReview",
	} {
		t.Run(name, func(t *testing.T) {
			settings, err := (yaml.Loader{}).Load()
			if err != nil {
				t.Fatal(err)
			}
			reflect.ValueOf(settings).Elem().FieldByName(name).SetString("")
			if err := settings.Validate(); err == nil {
				t.Fatal("accepted missing value")
			}
		})
	}
	for name, change := range map[string]func(*domain.Settings){
		"text kind":      func(s *domain.Settings) { s.Library.TextKind = "" },
		"jpeg":           func(s *domain.Settings) { s.Library.MIMEJPEG = "" },
		"ogg":            func(s *domain.Settings) { s.Library.MIMEOggAudio = "" },
		"mp4":            func(s *domain.Settings) { s.Library.MIMEMP4Video = "" },
		"rating minimum": func(s *domain.Settings) { s.Library.MinimumRating = 0 },
		"rating maximum": func(s *domain.Settings) { s.Library.MaximumRating = 5 },
		"invalid route":  func(s *domain.Settings) { s.MediaRoutePrefix = ":invalid" },
		"relative route": func(s *domain.Settings) { s.MediaRoutePrefix = "media/" },
		"missing route":  func(s *domain.Settings) { s.PreviewRoutePrefix = "" },
		"query route":    func(s *domain.Settings) { s.PreviewRoutePrefix = "/preview/?x=/" },
		"overlap":        func(s *domain.Settings) { s.PreviewRoutePrefix = s.MediaRoutePrefix },
		"nested route":   func(s *domain.Settings) { s.PreviewRoutePrefix = s.MediaRoutePrefix + "preview/" },
		"events":         func(s *domain.Settings) { s.ShowReviews = s.SchedulesChanged },
	} {
		t.Run(name, func(t *testing.T) {
			settings, err := (yaml.Loader{}).Load()
			if err != nil {
				t.Fatal(err)
			}
			change(settings)
			if err := settings.Validate(); err == nil {
				t.Fatal("accepted invalid contract")
			}
		})
	}
}

func TestGeometryAndDefaultValidation(t *testing.T) {
	for _, name := range []string{"WidgetSize", "ReviewWidth", "ReviewHeight", "ReviewGap"} {
		settings, err := (yaml.Loader{}).Load()
		if err != nil {
			t.Fatal(err)
		}
		reflect.ValueOf(&settings.Layout).Elem().FieldByName(name).SetInt(0)
		if err := settings.Validate(); err == nil {
			t.Fatal("accepted missing geometry")
		}
	}
	settings, err := (yaml.Loader{}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Validate(); err != nil {
		t.Fatal(err)
	}
}
