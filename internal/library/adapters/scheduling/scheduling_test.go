package scheduling

import (
	"errors"
	"testing"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"

	"raven/internal/library"
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
	scheduler := New()
	scheduler.engine = failingEngine{failure: failure}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	if _, _, err := scheduler.Initialize(now); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, _, err := scheduler.Next(library.Schedule{}, now, 3); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := scheduler.Validate(library.Schedule{}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
