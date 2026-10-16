package library_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	reviewapi "raven/api/reviews"
	"raven/internal/library"
)

type fakeScheduler struct{ failure error }

func (s fakeScheduler) Initialize(now time.Time) (reviewapi.Schedule, reviewapi.Log, error) {
	return reviewapi.Schedule{
		Due:        now,
		LastReview: now,
		Reps:       1,
	}, reviewapi.Log{
		Review: now,
		Rating: 1,
	}, s.failure
}

func (s fakeScheduler) Next(
	schedule *reviewapi.Schedule,
	now time.Time,
	rating int,
) (reviewapi.Schedule, reviewapi.Log, error) {
	// A failed scheduler must not mutate the caller's persisted state.
	if s.failure != nil {
		schedule.Reps++
	}
	before := *schedule
	before.Due = now.Add(time.Hour)
	before.LastReview = now
	before.Reps++
	return before, reviewapi.Log{Review: now, Rating: int8(rating)}, s.failure
}
func (s fakeScheduler) Validate(*reviewapi.Schedule) error { return s.failure }

func TestReviewFailureLeavesItemUnchanged(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	item := contentapi.Item{ID: "item", Kind: "text", Text: "Remember"}
	if err := library.InitializeItem(&item, now, fakeScheduler{}); err != nil {
		t.Fatal(err)
	}
	before := item
	failure := errors.New("schedule failed")
	if err := library.ApplyReview(
		&item,
		&reviewapi.Attempt{At: now, Rating: 3},
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
	if err := library.ApplyReview(
		&item,
		&reviewapi.Attempt{At: now, Rating: 5},
		fakeScheduler{},
	); err == nil {
		t.Fatal("accepted invalid rating")
	}
	if err := library.ApplyReview(
		&item,
		&reviewapi.Attempt{At: now, Rating: 3},
		fakeScheduler{},
	); err != nil {
		t.Fatal(err)
	}
	if item.Schedule.Reps != 2 || len(item.Reviews) != 2 || item.Reviews[1].Automatic {
		t.Fatal("review history changed")
	}
}

func TestPreparationReturnsRollbackFilesAndProtectsOwnership(t *testing.T) {
	imports := 0
	importer := func(original *contentapi.Item, source string) (contentapi.Item, string, error) {
		item := *original
		imports++
		item.RelativePath = "media/" + item.ID + ".png"
		item.OriginalName = source
		item.MIMEType = "image/png"
		item.ByteSize = 1
		return item, "copied.png", nil
	}
	id := func() (string, error) { return "0123456789abcdef0123456789abcdef", nil }
	_, rollback, err := library.PrepareItems([]contentapi.Draft{
		{Kind: "image", SourcePath: "image.png"},
		{ID: "foreign", Kind: "text", Text: "Changed"},
	}, nil, &library.PreparationDependencies{ImportMedia: importer, NewID: id})
	if err == nil || imports != 1 || !reflect.DeepEqual(rollback, []string{"copied.png"}) {
		t.Fatalf("rollback=%v imports=%d error=%v", rollback, imports, err)
	}
	old := contentapi.Item{
		ID:       "owned",
		Kind:     "text",
		Text:     "Before",
		Schedule: reviewapi.Schedule{Reps: 4},
	}
	items, _, err := library.PrepareItems(
		[]contentapi.Draft{{ID: "owned", Kind: "text", Text: " After "}},
		[]contentapi.Item{old},
		&library.PreparationDependencies{ImportMedia: importer, NewID: id},
	)
	if err != nil || items[0].Text != "After" || items[0].Schedule.Reps != 4 ||
		old.Text != "Before" {
		t.Fatalf("prepared=%+v error=%v", items, err)
	}
}

func TestHydrationOrdersItemsAndIgnoresAutomaticReviews(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	card := cardapi.Card{Items: []contentapi.Item{
		{
			ID:       "second",
			Position: 1,
			Schedule: reviewapi.Schedule{Due: now.Add(time.Hour)},
			Reviews:  []contentapi.Review{{Log: reviewapi.Log{Review: now}, Automatic: true}},
		},
		{
			ID:       "first",
			Position: 0,
			Schedule: reviewapi.Schedule{Due: now},
			Reviews:  []contentapi.Review{{Log: reviewapi.Log{Review: now.Add(-time.Hour)}}},
		},
	}}
	result, err := library.Hydrate(&card)
	if err != nil || result.Items[0].ID != "first" || result.DueAt != now.UnixMilli() ||
		result.LastReviewed != now.Add(-time.Hour).UnixMilli() {
		t.Fatalf("card=%+v error=%v", result, err)
	}
}

func TestFolderNormalizationAndContentValidation(t *testing.T) {
	folder, err := library.NormalizeFolder(
		&folderapi.CreateInput{
			Title:       " Ideas ",
			Description: " Notes ",
			Tags:        []string{" One ", "one", "", "Two"},
		},
	)
	if err != nil || folder.Title != "Ideas" || folder.Description != "Notes" ||
		!reflect.DeepEqual(folder.Tags, []string{"One", "Two"}) {
		t.Fatalf("normalization: %+v %v", folder, err)
	}
	if _, err := library.NormalizeFolder(&folderapi.CreateInput{}); err == nil {
		t.Fatal("accepted empty title")
	}
	id := "0123456789abcdef0123456789abcdef"
	valid := contentapi.Item{
		ID:           id,
		Kind:         "image",
		OriginalName: "image.png",
		MIMEType:     "image/png",
		RelativePath: "media/" + id + ".png",
		ByteSize:     1,
	}
	if err := library.ValidateItem(&valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*contentapi.Item){
		func(item *contentapi.Item) { item.ID = "" },
		func(item *contentapi.Item) { item.Position = -1 },
		func(item *contentapi.Item) { item.Kind = "invalid" },
		func(item *contentapi.Item) { item.Text = "unexpected" },
		func(item *contentapi.Item) { item.OriginalName = "" },
		func(item *contentapi.Item) { item.ByteSize = 0 },
		func(item *contentapi.Item) { item.MIMEType = "audio/wav" },
		func(item *contentapi.Item) { item.RelativePath = "../image.png" },
	} {
		invalid := valid
		mutate(&invalid)
		if err := library.ValidateItem(&invalid); err == nil {
			t.Fatalf("accepted invalid media: %+v", invalid)
		}
	}
	text := contentapi.Item{ID: "text", Kind: "text", Text: "Remember"}
	if err := library.ValidateItem(&text); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*contentapi.Item){
		func(item *contentapi.Item) { item.Text = " " },
		func(item *contentapi.Item) { item.RelativePath = "media/file.png" },
		func(item *contentapi.Item) { item.MIMEType = "image/png" },
		func(item *contentapi.Item) { item.OriginalName = "file.png" },
		func(item *contentapi.Item) { item.ByteSize = 1 },
	} {
		invalid := text
		mutate(&invalid)
		if err := library.ValidateItem(&invalid); err == nil {
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
	importer := func(original *contentapi.Item, _ string) (contentapi.Item, string, error) {
		item := *original
		return item, "", failure
	}
	if _, _, err := library.PrepareItems(
		nil,
		nil,
		&library.PreparationDependencies{ImportMedia: importer, NewID: newID},
	); err == nil {
		t.Fatal("accepted empty content")
	}
	if _, _, err := library.PrepareItems(
		[]contentapi.Draft{{Kind: "text", Text: "remember"}},
		nil,
		&library.PreparationDependencies{
			ImportMedia: importer,
			NewID:       func() (string, error) { return "", failure },
		},
	); !errors.Is(
		err,
		failure,
	) {
		t.Fatal(err)
	}
	if _, _, err := library.PrepareItems(
		[]contentapi.Draft{{Kind: "image", SourcePath: "image.png"}},
		nil,
		&library.PreparationDependencies{ImportMedia: importer, NewID: newID},
	); !errors.Is(
		err,
		failure,
	) {
		t.Fatal(err)
	}
	for _, draft := range []contentapi.Draft{{Kind: "text", Text: "remember", SourcePath: "invalid"}, {Kind: "image", Text: "unexpected"}, {Kind: "text", Text: " "}} {
		if _, _, err := library.PrepareItems(
			[]contentapi.Draft{draft},
			nil,
			&library.PreparationDependencies{ImportMedia: importer, NewID: newID},
		); err == nil {
			t.Fatalf("accepted draft: %+v", draft)
		}
	}
	previous := []contentapi.Item{{ID: id, Kind: "text", Text: "before"}}
	media := []contentapi.Item{{
		ID: id, Kind: "image", OriginalName: "image.png", MIMEType: "image/png",
		RelativePath: "media/" + id + ".png", ByteSize: 1,
	}}
	if _, _, err := library.PrepareItems(
		[]contentapi.Draft{{ID: id, Kind: "image", Text: "unexpected"}},
		media,
		&library.PreparationDependencies{ImportMedia: importer, NewID: newID},
	); err == nil ||
		!strings.Contains(err.Error(), "media cannot contain text") {
		t.Fatalf("accepted text on retained media: %v", err)
	}
	for _, drafts := range [][]contentapi.Draft{
		{{ID: id, Kind: "image"}},
		{{ID: id, Kind: "text", Text: "after", SourcePath: "invalid"}},
		{{ID: id, Kind: "text", Text: "after"}, {ID: id, Kind: "text", Text: "duplicate"}},
	} {
		if _, _, err := library.PrepareItems(
			drafts,
			previous,
			&library.PreparationDependencies{ImportMedia: importer, NewID: newID},
		); err == nil {
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
	if _, err := library.Hydrate(&cardapi.Card{}); err == nil {
		t.Fatal("accepted empty card")
	}
	if _, err := library.Hydrate(
		&cardapi.Card{Items: []contentapi.Item{{Position: 1}}},
	); err == nil {
		t.Fatal("accepted unordered content")
	}
}
