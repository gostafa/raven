package fsrs_test

import (
	"testing"
	"time"

	contentapi "raven/api/content"
	reviewapi "raven/api/reviews"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
	scheduling "raven/internal/shared/scheduling/adapters/fsrs"
)

func TestSchedulesAdvanceAndValidateAcrossRoundTrips(t *testing.T) {
	scheduler := scheduling.New(new(configuration.Library(configurationFixture())))
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	scheduleValue, initialValue, err := scheduler.Initialize(now)
	schedule := decodeSchedule(t, scheduleValue)
	initial := decodeLog(t, initialValue)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Rating != 1 || schedule.Reps != 1 || !initial.Review.Equal(now) {
		t.Fatalf("unexpected initialization: %+v %+v", schedule, initial)
	}
	if err := scheduler.Validate(&schedule); err != nil {
		t.Fatal(err)
	}
	for _, rating := range []int{1, 2, 3, 4} {
		updatedValue, logValue, err := scheduler.Next(&schedule, now.Add(time.Minute), rating)
		updated := decodeSchedule(t, updatedValue)
		log := decodeLog(t, logValue)
		if err != nil {
			t.Fatal(err)
		}
		if log.Rating != int8(rating) || updated.Reps != 2 ||
			!updated.LastReview.Equal(log.Review) {
			t.Fatalf("lost scheduling state: %+v %+v", updated, log)
		}
		if err := scheduler.Validate(&updated); err != nil {
			t.Fatal(err)
		}
	}
	for _, rating := range []int{0, 5} {
		if _, _, err := scheduler.Next(&schedule, now, rating); err == nil {
			t.Fatalf("accepted rating %d", rating)
		}
	}
	schedule.State = -1
	if err := scheduler.Validate(&schedule); err == nil {
		t.Fatal("accepted invalid schedule state")
	}
	if _, _, err := scheduler.Next(&schedule, now, 3); err == nil {
		t.Fatal("advanced invalid schedule state")
	}
	if _, _, err := scheduler.Initialize(time.Time{}); err != nil {
		t.Fatalf("zero time compatibility: %v", err)
	}
}

func configurationFixture() configuration.Settings {
	settings, err := configyaml.Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func decodeSchedule(t *testing.T, value any) reviewapi.Schedule {
	t.Helper()
	schedule, err := contentapi.Decode[reviewapi.Schedule](value)
	if err != nil {
		t.Fatal(err)
	}
	return schedule
}

func decodeLog(t *testing.T, value any) reviewapi.Log {
	t.Helper()
	log, err := contentapi.Decode[reviewapi.Log](value)
	if err != nil {
		t.Fatal(err)
	}
	return log
}
