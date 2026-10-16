package scheduling_test

import (
	"testing"
	"time"

	"raven/internal/library/adapters/scheduling"
)

func TestSchedulesAdvanceAndValidateAcrossRoundTrips(t *testing.T) {
	scheduler := scheduling.New()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	schedule, initial, err := scheduler.Initialize(now)
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
		updated, log, err := scheduler.Next(&schedule, now.Add(time.Minute), rating)
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
