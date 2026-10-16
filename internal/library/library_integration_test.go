package library_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"raven/internal/library"
)

type fakeScheduler struct{ failure error }

func (s fakeScheduler) Initialize(now time.Time) (library.Schedule, library.ReviewLog, error) {
	return library.Schedule{
		Due:        now,
		LastReview: now,
		Reps:       1,
	}, library.ReviewLog{
		Review: now,
		Rating: 1,
	}, s.failure
}

func (s fakeScheduler) Next(
	before library.Schedule,
	now time.Time,
	rating int,
) (library.Schedule, library.ReviewLog, error) {
	before.Due = now.Add(time.Hour)
	before.LastReview = now
	before.Reps++
	return before, library.ReviewLog{Review: now, Rating: int8(rating)}, s.failure
}
func (s fakeScheduler) Validate(library.Schedule) error { return s.failure }

func TestReviewFailureLeavesItemUnchanged(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	item := library.ContentItem{ID: "item", Kind: "text", Text: "Remember"}
	if err := library.InitializeItem(&item, now, fakeScheduler{}); err != nil {
		t.Fatal(err)
	}
	before := item
	failure := errors.New("schedule failed")
	if err := library.ApplyReview(
		&item,
		now,
		3,
		fakeScheduler{failure: failure},
	); !errors.Is(
		err,
		failure,
	) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(item, before) {
		t.Fatal("failed review mutated item")
	}
	if err := library.ApplyReview(&item, now, 5, fakeScheduler{}); err == nil {
		t.Fatal("accepted invalid rating")
	}
	if err := library.ApplyReview(&item, now, 3, fakeScheduler{}); err != nil {
		t.Fatal(err)
	}
	if item.Schedule.Reps != 2 || len(item.Reviews) != 2 || item.Reviews[1].Automatic {
		t.Fatal("review history changed")
	}
}

func TestPreparationReturnsRollbackFilesAndProtectsOwnership(t *testing.T) {
	imports := 0
	importer := func(item library.ContentItem, source string) (library.ContentItem, string, error) {
		imports++
		item.RelativePath = "media/" + item.ID + ".png"
		item.OriginalName = source
		item.MIMEType = "image/png"
		item.ByteSize = 1
		return item, "copied.png", nil
	}
	id := func() (string, error) { return "0123456789abcdef0123456789abcdef", nil }
	_, rollback, err := library.PrepareItems(
		[]library.ItemDraft{
			{Kind: "image", SourcePath: "image.png"},
			{ID: "foreign", Kind: "text", Text: "Changed"},
		},
		nil,
		importer,
		id,
	)
	if err == nil || imports != 1 || !reflect.DeepEqual(rollback, []string{"copied.png"}) {
		t.Fatalf("rollback=%v imports=%d error=%v", rollback, imports, err)
	}
	old := library.ContentItem{
		ID:       "owned",
		Kind:     "text",
		Text:     "Before",
		Schedule: library.Schedule{Reps: 4},
	}
	items, _, err := library.PrepareItems(
		[]library.ItemDraft{{ID: "owned", Kind: "text", Text: " After "}},
		[]library.ContentItem{old},
		importer,
		id,
	)
	if err != nil || items[0].Text != "After" || items[0].Schedule.Reps != 4 ||
		old.Text != "Before" {
		t.Fatalf("prepared=%+v error=%v", items, err)
	}
}

func TestHydrationOrdersItemsAndIgnoresAutomaticReviews(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	card := library.Card{Items: []library.ContentItem{
		{
			ID:       "second",
			Position: 1,
			Schedule: library.Schedule{Due: now.Add(time.Hour)},
			Reviews:  []library.ItemReview{{Log: library.ReviewLog{Review: now}, Automatic: true}},
		},
		{
			ID:       "first",
			Position: 0,
			Schedule: library.Schedule{Due: now},
			Reviews:  []library.ItemReview{{Log: library.ReviewLog{Review: now.Add(-time.Hour)}}},
		},
	}}
	result, err := library.Hydrate(card)
	if err != nil || result.Items[0].ID != "first" || result.DueAt != now.UnixMilli() ||
		result.LastReviewed != now.Add(-time.Hour).UnixMilli() {
		t.Fatalf("card=%+v error=%v", result, err)
	}
}
