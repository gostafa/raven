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
	scheduler := newScheduler(
		failingEngine{failure: failure},
		new(configuration.Library(configurationFixture())),
	)
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
	scheduler := newScheduler(failingEngine{}, new(configuration.Library(configurationFixture())))
	if err := scheduler.Validate(&reviewapi.Schedule{}); !errors.Is(err, errIncompleteSchedules) {
		t.Fatalf("accepted missing rating schedules: %v", err)
	}
}

func configurationFixture() configuration.Settings {
	settings, err := configyaml.Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func TestRatingTranslationRejectsUnsupportedValues(t *testing.T) {
	if _, err := ratingFor(5); err == nil {
		t.Fatal("accepted unsupported FSRS rating")
	}
	settings := configuration.Library(configurationFixture())
	settings.MaximumRating = 5
	scheduler := New(&settings)
	if _, _, err := scheduler.Initialize(time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := scheduler.Next(&reviewapi.Schedule{}, time.Now(), 5); err == nil {
		t.Fatal("accepted unsupported configured rating")
	}
}

func TestSchedulingRejectsValuesOutsideTheWireSchema(t *testing.T) {
	scheduler := New(new(configuration.Library(configurationFixture())))
	for _, invalid := range []any{42, "unexpected", []string{"not", "a schedule"}, map[string]any{"due": false}} {
		if err := scheduler.Validate(invalid); err == nil {
			t.Fatalf("validated invalid schedule: %#v", invalid)
		}
		if _, _, err := scheduler.Next(invalid, time.Now(), 3); err == nil {
			t.Fatalf("advanced invalid schedule: %#v", invalid)
		}
	}
}

func TestSchedulingWireRejectsUnrepresentableTimestamps(t *testing.T) {
	invalid := time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)
	if _, _, err := encodeSchedule(
		&reviewapi.Schedule{Due: invalid},
		&reviewapi.Log{},
	); err == nil {
		t.Fatal("encoded a scheduling timestamp outside JSON's supported range")
	}
	if _, _, err := encodeSchedule(
		&reviewapi.Schedule{},
		&reviewapi.Log{Review: invalid},
	); err == nil {
		t.Fatal("encoded a review timestamp outside JSON's supported range")
	}
}
