package fsrs

import (
	"errors"
	"testing"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"

	reviewapi "raven/api/reviews"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
)

type failingEngine struct{ failure error }

func (engine failingEngine) Next(fsrs.Card, time.Time, fsrs.Rating) (fsrs.SchedulingInfo, error) {
	return fsrs.SchedulingInfo{}, engine.failure
}

func (engine failingEngine) Repeat(fsrs.Card, time.Time) (fsrs.RecordLog, error) {
	return fsrs.RecordLog{}, engine.failure
}

func TestEngineFailuresPreserveTheirCause(t *testing.T) {
	failure := errors.New("scheduling engine failed")
	scheduler := New(&configurationFixture().Library)
	scheduler.engine = failingEngine{failure: failure}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	if _, _, err := scheduler.Initialize(now); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, _, err := scheduler.Next(&reviewapi.Schedule{}, now, 3); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := scheduler.Validate(&reviewapi.Schedule{}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}

func TestValidationRejectsAnIncompleteRatingSet(t *testing.T) {
	scheduler := Scheduler{rules: &configurationFixture().Library, engine: failingEngine{}}
	if err := scheduler.Validate(&reviewapi.Schedule{}); !errors.Is(err, errIncompleteSchedules) {
		t.Fatalf("accepted missing rating schedules: %v", err)
	}
}

func configurationFixture() *configuration.Settings {
	settings, err := (configyaml.Loader{}).Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func TestRatingTranslationRejectsUnsupportedValues(t *testing.T) {
	if _, err := ratingFor(5); err == nil {
		t.Fatal("accepted unsupported FSRS rating")
	}
	settings := configurationFixture().Library
	settings.MaximumRating = 5
	scheduler := New(&settings)
	if _, _, err := scheduler.Initialize(time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := scheduler.Next(&reviewapi.Schedule{}, time.Now(), 5); err == nil {
		t.Fatal("accepted unsupported configured rating")
	}
}
