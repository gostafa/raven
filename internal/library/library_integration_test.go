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

func TestFolderNormalizationAndContentValidation(t *testing.T) {
	folder, err := library.NormalizeFolder(
		library.CreateFolderInput{
			Title:       " Ideas ",
			Description: " Notes ",
			Tags:        []string{" One ", "one", "", "Two"},
		},
	)
	if err != nil || folder.Title != "Ideas" || folder.Description != "Notes" ||
		!reflect.DeepEqual(folder.Tags, []string{"One", "Two"}) {
		t.Fatalf("normalization: %+v %v", folder, err)
	}
	if _, err := library.NormalizeFolder(library.CreateFolderInput{}); err == nil {
		t.Fatal("accepted empty title")
	}
	id := "0123456789abcdef0123456789abcdef"
	valid := library.ContentItem{
		ID:           id,
		Kind:         "image",
		OriginalName: "image.png",
		MIMEType:     "image/png",
		RelativePath: "media/" + id + ".png",
		ByteSize:     1,
	}
	if err := library.ValidateItem(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*library.ContentItem){
		func(item *library.ContentItem) { item.ID = "" },
		func(item *library.ContentItem) { item.Position = -1 },
		func(item *library.ContentItem) { item.Kind = "invalid" },
		func(item *library.ContentItem) { item.Text = "unexpected" },
		func(item *library.ContentItem) { item.OriginalName = "" },
		func(item *library.ContentItem) { item.ByteSize = 0 },
		func(item *library.ContentItem) { item.MIMEType = "audio/wav" },
		func(item *library.ContentItem) { item.RelativePath = "../image.png" },
	} {
		invalid := valid
		mutate(&invalid)
		if err := library.ValidateItem(invalid); err == nil {
			t.Fatalf("accepted invalid media: %+v", invalid)
		}
	}
	text := library.ContentItem{ID: "text", Kind: "text", Text: "Remember"}
	if err := library.ValidateItem(text); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*library.ContentItem){
		func(item *library.ContentItem) { item.Text = " " },
		func(item *library.ContentItem) { item.RelativePath = "media/file.png" },
		func(item *library.ContentItem) { item.MIMEType = "image/png" },
		func(item *library.ContentItem) { item.OriginalName = "file.png" },
		func(item *library.ContentItem) { item.ByteSize = 1 },
	} {
		invalid := text
		mutate(&invalid)
		if err := library.ValidateItem(invalid); err == nil {
			t.Fatal("accepted invalid text")
		}
	}
	for _, extension := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".mp3", ".wav", ".m4a", ".ogg", ".oga", ".flac", ".mp4", ".m4v", ".webm", ".ogv", ".mov"} {
		if library.MediaType(extension) == "" ||
			!library.ValidMediaPath("media/"+id+extension, id) {
			t.Fatalf("lost supported media %s", extension)
		}
	}
	if library.MediaType(".svg") != "" || library.ValidMediaPath("media/"+id+".svg", id) ||
		library.ValidMediaPath("media/bad.png", "bad") {
		t.Fatal("accepted unsupported media path")
	}
}

func TestPreparationFailuresRetainRollbackAndExistingRecords(t *testing.T) {
	id := "0123456789abcdef0123456789abcdef"
	failure := errors.New("dependency failed")
	newID := func() (string, error) { return id, nil }
	importer := func(item library.ContentItem, _ string) (library.ContentItem, string, error) {
		return item, "", failure
	}
	if _, _, err := library.PrepareItems(nil, nil, importer, newID); err == nil {
		t.Fatal("accepted empty content")
	}
	if _, _, err := library.PrepareItems(
		[]library.ItemDraft{{Kind: "text", Text: "remember"}},
		nil,
		importer,
		func() (string, error) { return "", failure },
	); !errors.Is(
		err,
		failure,
	) {
		t.Fatal(err)
	}
	if _, _, err := library.PrepareItems(
		[]library.ItemDraft{{Kind: "image", SourcePath: "image.png"}},
		nil,
		importer,
		newID,
	); !errors.Is(
		err,
		failure,
	) {
		t.Fatal(err)
	}
	for _, draft := range []library.ItemDraft{{Kind: "text", Text: "remember", SourcePath: "invalid"}, {Kind: "image", Text: "unexpected"}, {Kind: "text", Text: " "}} {
		if _, _, err := library.PrepareItems(
			[]library.ItemDraft{draft},
			nil,
			importer,
			newID,
		); err == nil {
			t.Fatalf("accepted draft: %+v", draft)
		}
	}
	previous := []library.ContentItem{{ID: id, Kind: "text", Text: "before"}}
	for _, drafts := range [][]library.ItemDraft{
		{{ID: id, Kind: "image"}},
		{{ID: id, Kind: "text", Text: "after", SourcePath: "invalid"}},
		{{ID: id, Kind: "text", Text: "after"}, {ID: id, Kind: "text", Text: "duplicate"}},
	} {
		if _, _, err := library.PrepareItems(drafts, previous, importer, newID); err == nil {
			t.Fatal("accepted invalid ownership or duplicate")
		}
	}
	item := previous[0]
	if err := library.InitializeItem(
		&item,
		time.Now(),
		fakeScheduler{failure: failure},
	); !errors.Is(
		err,
		failure,
	) {
		t.Fatal(err)
	}
	if _, err := library.Hydrate(library.Card{}); err == nil {
		t.Fatal("accepted empty card")
	}
	if _, err := library.Hydrate(
		library.Card{Items: []library.ContentItem{{Position: 1}}},
	); err == nil {
		t.Fatal("accepted unordered content")
	}
}
