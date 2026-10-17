package boltstore

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/multierr"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	reviewapi "raven/api/reviews"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
	"raven/internal/features/library/adapters/cffbatch"
	"raven/internal/features/library/adapters/scheduling"
)

func TestCardsPersistWithReviewSchedule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	folder, err := store.CreateFolder(&folderapi.CreateInput{Title: "Spanish"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := store.CreateCard(&cardapi.CreateInput{
		FolderID: folder.ID,
		Items:    textItems("  See you tomorrow? ", " Hasta mañana. "),
	})
	if err != nil {
		t.Fatal(err)
	}
	if card.Items[0].Text != "See you tomorrow?" || card.Items[1].Text != "Hasta mañana." ||
		card.LastReviewed != 0 {
		t.Fatalf("unexpected new card: %+v", card)
	}
	initial := card.Items[0]
	expected, err := fsrs.NewFSRS(fsrs.DefaultParam()).
		Next(fsrs.NewCard(initial.Schedule.LastReview), initial.Schedule.LastReview, fsrs.Again)
	if err != nil || !reflect.DeepEqual(nativeSchedule(initial.Schedule), expected.Card) ||
		!initial.Reviews[0].Automatic {
		t.Fatal("initialization differs from FSRS Again")
	}
	sibling := card.Items[1]
	now := initial.Schedule.Due
	for _, rating := range []int{1, 2, 3, 4} {
		before := card.Items[0].Schedule
		expected, err := fsrs.NewFSRS(fsrs.DefaultParam()).
			Next(nativeSchedule(before), now, fsrs.Rating(rating))
		if err != nil {
			t.Fatal(err)
		}
		card, err = store.reviewItemAt(
			&cardapi.ReviewInput{
				FolderID: folder.ID,
				CardID:   card.ID,
				ItemID:   initial.ID,
				Rating:   rating,
			},
			now,
		)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(nativeSchedule(card.Items[0].Schedule), expected.Card) ||
			!reflect.DeepEqual(card.Items[1], sibling) ||
			card.LastReviewed != now.UnixMilli() {
			t.Fatal("rating differs from FSRS or changed sibling")
		}
		now = expected.Card.Due
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := store.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card}) {
		t.Fatalf("cards after reopen: %+v, %v", cards, err)
	}
}

// Noncanonical JSON and a sequence marker reveal accidental bucket replacement
// and rewriting of records that do not belong to an update.
func markItemBucket(t *testing.T, store *Store, card *cardapi.Card) map[string][]byte {
	t.Helper()
	err := store.db.Update(func(tx *bolt.Tx) error {
		bucket := testItemBucket(tx, card)
		if err := bucket.SetSequence(42); err != nil {
			return err
		}
		for index := range card.Items {
			item := &card.Items[index]
			data := append([]byte(" \n"), bucket.Get([]byte(item.ID))...)
			if err := bucket.Put([]byte(item.ID), data); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshotItemRecords(t, store, card)
}

func testItemBucket(tx *bolt.Tx, card *cardapi.Card) *bolt.Bucket {
	return tx.Bucket([]byte(foldersBucket)).Bucket([]byte(card.FolderID)).
		Bucket([]byte(itemsBucket)).Bucket([]byte(card.ID))
}

func snapshotItemRecords(t *testing.T, store *Store, card *cardapi.Card) map[string][]byte {
	t.Helper()
	records := make(map[string][]byte)
	err := store.db.View(func(tx *bolt.Tx) error {
		bucket := testItemBucket(tx, card)
		if bucket.Sequence() != 42 {
			t.Error("item bucket sequence changed")
		}
		return bucket.ForEach(func(key, value []byte) error {
			records[string(key)] = bytes.Clone(value)
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func TestReviewWritesOnlyOwnedItemAndReturnsHydratedCard(t *testing.T) {
	store, cards := persistenceFixture(t, 3)
	card := &cards[0]
	before := markItemBucket(t, store, card)
	item := &card.Items[0]
	now := item.Schedule.Due.Add(time.Second)
	updated, err := store.reviewItemAt(&cardapi.ReviewInput{
		FolderID: card.FolderID, CardID: card.ID, ItemID: item.ID, Rating: 4,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	after := snapshotItemRecords(t, store, card)
	for index := range card.Items {
		id := card.Items[index].ID
		if bytes.Equal(before[id], after[id]) != (index != 0) {
			t.Fatalf("unexpected record change for item %d", index)
		}
	}
	if updated.LastReviewed != now.UnixMilli() ||
		updated.DueAt != card.Items[1].Schedule.Due.UnixMilli() {
		t.Fatal("returned aggregate schedule is stale")
	}
	listed, err := store.ListCards()
	if err != nil || !reflect.DeepEqual(listed[0], updated) {
		t.Fatalf("returned card differs from stored card: %v", err)
	}
	assertReopenedCards(t, store, listed)
}

func assertReopenedCards(t *testing.T, store *Store, expected []cardapi.Card) {
	t.Helper()
	path := store.db.Path()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	actual, err := reopened.ListCards()
	if err != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("cards changed after reopening: %v", err)
	}
}

func notificationOccurrence(card *cardapi.Card, index int) reviewapi.DueOccurrence {
	item := &card.Items[index]
	return reviewapi.DueOccurrence{
		FolderID: card.FolderID,
		CardID:   card.ID,
		ItemID:   item.ID,
		Due:      item.Schedule.Due.UnixMilli(),
	}
}

func TestNotificationBatchWritesOnlyChangedItems(t *testing.T) {
	store, cards := persistenceFixture(t, 3)
	before := make([]map[string][]byte, len(cards))
	for index := range cards {
		before[index] = markItemBucket(t, store, &cards[index])
	}
	first := notificationOccurrence(&cards[0], 0)
	other := notificationOccurrence(&cards[1], 0)
	second := notificationOccurrence(&cards[0], 1)
	stale := notificationOccurrence(&cards[0], 2)
	stale.Due--
	missing := first
	missing.ItemID = "missing"
	batch := []reviewapi.DueOccurrence{first, other, first, stale, missing, second}
	if err := store.MarkNotified(batch); err != nil {
		t.Fatal(err)
	}
	for index := range cards {
		after := snapshotItemRecords(t, store, &cards[index])
		for position, item := range cards[index].Items {
			changed := (index == 0 && position < 2) || (index == 1 && position == 0)
			if bytes.Equal(before[index][item.ID], after[item.ID]) == changed {
				t.Fatalf("unexpected change for card %d item %d", index, position)
			}
		}
	}
	err := store.db.Update(func(tx *bolt.Tx) error {
		if err := store.markOccurrences(tx, batch); err != nil {
			return err
		}
		stats := tx.Stats()
		if stats.GetNodeCount() != 0 {
			t.Error("no-op acknowledgment batch dirtied records")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := store.ListCards()
	if err != nil {
		t.Fatal(err)
	}
	if listed[0].Items[0].LastNotifiedDue != first.Due ||
		listed[0].Items[1].LastNotifiedDue != second.Due ||
		listed[1].Items[0].LastNotifiedDue != other.Due {
		t.Fatal("acknowledgments did not persist")
	}
	assertReopenedCards(t, store, listed)
}

func TestNotificationBatchRollsBackEarlierCards(t *testing.T) {
	for _, missingFolder := range []bool{false, true} {
		t.Run(fmt.Sprintf("missingFolder=%t", missingFolder), func(t *testing.T) {
			store, cards := persistenceFixture(t, 2)
			before := markItemBucket(t, store, &cards[0])
			missing := notificationOccurrence(&cards[1], 0)
			missing.CardID = "missing"
			expected := errCardNotFound
			if missingFolder {
				missing.FolderID = "missing"
				expected = errFolderNotFoundOrInvalid
			}
			err := store.MarkNotified([]reviewapi.DueOccurrence{
				notificationOccurrence(&cards[0], 0), missing,
			})
			if !errors.Is(err, expected) {
				t.Fatalf("unexpected missing-record error: %v", err)
			}
			if !reflect.DeepEqual(before, snapshotItemRecords(t, store, &cards[0])) {
				t.Fatal("failed batch committed earlier acknowledgment")
			}
		})
	}
}

func TestTargetedUpdatesStillValidateSiblingContent(t *testing.T) {
	for _, operation := range []string{"review", "acknowledge"} {
		t.Run(operation, func(t *testing.T) {
			store, cards := persistenceFixture(t, 2)
			card := &cards[0]
			markItemBucket(t, store, card)
			if err := store.db.Update(func(tx *bolt.Tx) error {
				return testItemBucket(tx, card).Put([]byte(card.Items[1].ID), []byte("invalid"))
			}); err != nil {
				t.Fatal(err)
			}
			before := snapshotItemRecords(t, store, card)
			var err error
			if operation == "review" {
				_, err = store.ReviewItem(&cardapi.ReviewInput{
					FolderID: card.FolderID, CardID: card.ID, ItemID: card.Items[0].ID, Rating: 3,
				})
			} else {
				err = store.MarkNotified([]reviewapi.DueOccurrence{notificationOccurrence(card, 0)})
			}
			if err == nil {
				t.Fatal("corrupt sibling content was ignored")
			}
			if !reflect.DeepEqual(before, snapshotItemRecords(t, store, card)) {
				t.Fatal("failed update changed item records")
			}
		})
	}
}

func TestManagedStorageDefersOpeningAndGatesMedia(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store := New(
		path,
		&Dependencies{
			Batch:     cffbatch.New(),
			Scheduler: scheduling.New(&configurationFixture().Library),
			Logger:    storageLogger(nil),
			Settings:  *storeSettingsFixture(),
		},
	)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("constructor opened database: %v", err)
	}
	checkUnavailable := func() {
		t.Helper()
		response := httptest.NewRecorder()
		store.MediaHandler().
			ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/media/folder/card/item", nil))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("inactive media returned %d", response.Code)
		}
	}
	checkUnavailable()
	if err := store.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Start(); !errors.Is(err, errStoreAlreadyStarted) {
		t.Fatalf("repeated startup: %v", err)
	}
	if _, err := store.ListFolders(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	checkUnavailable()
	reopened, err := Open(
		path,
		&Dependencies{
			Batch:     cffbatch.New(),
			Scheduler: scheduling.New(&configurationFixture().Library),
			Logger:    nil,
			Settings:  *storeSettingsFixture(),
		},
	)
	if err != nil {
		t.Fatalf("managed shutdown retained lock: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFailedPreparationReleasesManagedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		metadata, err := tx.CreateBucketIfNotExists([]byte("meta"))
		if err != nil {
			return err
		}
		return metadata.Put([]byte("schemaVersion"), []byte("999"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store := New(
		path,
		&Dependencies{
			Batch:     cffbatch.New(),
			Scheduler: scheduling.New(&configurationFixture().Library),
			Logger:    storageLogger(nil),
			Settings:  *storeSettingsFixture(),
		},
	)
	if err := store.Start(); err == nil {
		t.Fatal("accepted unsupported schema")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		t.Fatalf("failed preparation retained database lock: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCardsValidateAndStayInTheirFolder(t *testing.T) {
	store := openTestStore(t)
	first, _ := store.CreateFolder(&folderapi.CreateInput{Title: "First"})
	second, _ := store.CreateFolder(&folderapi.CreateInput{Title: "Second"})
	for _, input := range []cardapi.CreateInput{
		{FolderID: first.ID, Items: textItems(" ", "answer")},
		{FolderID: first.ID, Items: textItems("question", " \n")},
		{FolderID: "missing", Items: textItems("question", "answer")},
	} {
		if _, err := store.CreateCard(&input); err == nil {
			t.Fatalf("accepted invalid card: %+v", input)
		}
	}
	cards, err := store.ListCards()
	if err != nil || cards == nil || len(cards) != 0 {
		t.Fatalf("failed save wrote cards: %+v, %v", cards, err)
	}
	card, err := store.CreateCard(
		&cardapi.CreateInput{FolderID: first.ID, Items: textItems("Question", "Answer")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewItem(
		&cardapi.ReviewInput{
			FolderID: second.ID,
			CardID:   card.ID,
			ItemID:   card.Items[0].ID,
			Rating:   3,
		},
	); err == nil {
		t.Fatal("reviewed a card through the wrong folder")
	}
	if _, err := store.ReviewItem(
		&cardapi.ReviewInput{
			FolderID: first.ID,
			CardID:   "missing",
			ItemID:   card.Items[0].ID,
			Rating:   3,
		},
	); err == nil {
		t.Fatal("reviewed a missing card")
	}
	cards, err = store.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card}) {
		t.Fatalf("failed review changed cards: %+v, %v", cards, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListCards(); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("closed list: %v", err)
	}
	if _, err := store.CreateCard(
		&cardapi.CreateInput{FolderID: first.ID, Items: textItems("Question", "Answer")},
	); !errors.Is(
		err,
		bolt.ErrDatabaseNotOpen,
	) {
		t.Fatalf("closed create: %v", err)
	}
	if _, err := store.ReviewItem(
		&cardapi.ReviewInput{
			FolderID: first.ID,
			CardID:   card.ID,
			ItemID:   card.Items[0].ID,
			Rating:   3,
		},
	); !errors.Is(
		err,
		bolt.ErrDatabaseNotOpen,
	) {
		t.Fatalf("closed review: %v", err)
	}
}

func textItems(texts ...string) []contentapi.Draft {
	items := make([]contentapi.Draft, 0, len(texts))
	for _, text := range texts {
		items = append(items, contentapi.Draft{Kind: "text", Text: text})
	}
	return items
}

// Tiny container signatures keep tests independent of codecs or external tools.
func mediaFixture(t *testing.T, ext string) (string, []byte) {
	t.Helper()
	var data []byte
	switch ext {
	case ".png":
		data = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	case ".wav":
		data = append([]byte("RIFF\x40\x00\x00\x00WAVE"), bytes.Repeat([]byte{0}, 64)...)
	case ".mp4", ".m4a":
		data = make([]byte, 32)
		binary.BigEndian.PutUint32(data, 32)
		copy(data[4:], "ftypisom")
		copy(data[16:], "isommp42")
	default:
		t.Fatal("unknown fixture extension")
	}
	path := filepath.Join(t.TempDir(), "source"+ext)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, data
}

func itemDrafts(card cardapi.Card) []contentapi.Draft {
	drafts := make([]contentapi.Draft, 0, len(card.Items))
	for _, item := range card.Items {
		drafts = append(drafts, contentapi.Draft{ID: item.ID, Kind: item.Kind, Text: item.Text})
	}
	return drafts
}

func TestMixedContentPersistenceEditingAndMediaCopies(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(&folderapi.CreateInput{Title: "Mixed"})
	drafts := []contentapi.Draft{}
	for batch := range 2 {
		for i := range 2 {
			drafts = append(
				drafts,
				contentapi.Draft{Kind: "text", Text: fmt.Sprintf(" text %d %d ", batch, i)},
			)
			for _, media := range []struct{ kind, ext string }{{"image", ".png"}, {"audio", ".wav"}, {"video", ".mp4"}} {
				path, _ := mediaFixture(t, media.ext)
				drafts = append(drafts, contentapi.Draft{Kind: media.kind, SourcePath: path})
			}
		}
	}
	card, err := s.CreateCard(&cardapi.CreateInput{FolderID: folder.ID, Items: drafts})
	if err != nil {
		t.Fatal(err)
	}
	if len(card.Items) != 16 {
		t.Fatalf("items: %+v", card.Items)
	}
	for i, item := range card.Items {
		if item.Position != i || item.FolderID != folder.ID || item.CardID != card.ID {
			t.Fatalf("relation/order: %+v", item)
		}
		if item.Kind != "text" {
			data, err := os.ReadFile(s.mediaPath(item.RelativePath))
			if err != nil || int64(len(data)) != item.ByteSize {
				t.Fatalf("managed copy: %v", err)
			}
			if err := os.Remove(drafts[i].SourcePath); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		bucket, _ := cardBucket(tx, folder.ID)
		data := bucket.Get([]byte(card.ID))
		if bytes.Contains(data, []byte(`"items"`)) || bytes.Contains(data, []byte(`"prompt"`)) {
			t.Fatalf("card embeds content: %s", data)
		}
		content := tx.Bucket([]byte(foldersBucket)).
			Bucket([]byte(folder.ID)).
			Bucket([]byte(itemsBucket)).
			Bucket([]byte(card.ID))
		if content.Stats().KeyN != 16 {
			t.Fatal("items are not separate records")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	card, err = s.ReviewItem(
		&cardapi.ReviewInput{
			FolderID: folder.ID,
			CardID:   card.ID,
			ItemID:   card.Items[0].ID,
			Rating:   3,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	due, reviewed := card.DueAt, card.LastReviewed
	edit := itemDrafts(card)
	removed := card.Items[1]
	edit = append(edit[:1], edit[2:]...)
	edit[0], edit[1] = edit[1], edit[0]
	edit[1].Text = "Updated text"
	updated, err := s.UpdateCard(
		&cardapi.UpdateInput{FolderID: folder.ID, CardID: card.ID, Items: edit},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DueAt != due || updated.LastReviewed != reviewed ||
		updated.CreatedAt != card.CreatedAt {
		t.Fatal("edit changed schedule")
	}
	if updated.Items[0].ID != edit[0].ID || updated.Items[1].Text != "Updated text" {
		t.Fatal("edit/reorder lost")
	}
	if _, err := os.Stat(s.mediaPath(removed.RelativePath)); !os.IsNotExist(err) {
		t.Fatal("removed media not cleaned")
	}
	path := s.db.Path()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cards, err := reopened.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{updated}) {
		t.Fatalf("reopen mismatch: %v", err)
	}
}

func TestContentValidationAndRollback(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(&folderapi.CreateInput{Title: "First"})
	other, _ := s.CreateFolder(&folderapi.CreateInput{Title: "Second"})
	card, err := s.CreateCard(
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("Prompt", "Answer")},
	)
	if err != nil {
		t.Fatal(err)
	}
	otherCard, err := s.CreateCard(
		&cardapi.CreateInput{FolderID: other.ID, Items: textItems("Other", "Answer")},
	)
	if err != nil {
		t.Fatal(err)
	}
	// cardapi.Card IDs are folder scoped, so both may be equal; content IDs still cannot cross ownership.
	foreign := itemDrafts(otherCard)
	duplicate := itemDrafts(card)
	duplicate = append(duplicate, duplicate[0])
	invalid := [][]contentapi.Draft{
		{},
		{{Kind: "text", Text: " "}},
		{{Kind: "html", Text: "Wrong kind"}},
		foreign, duplicate,
		{{ID: card.Items[0].ID, Kind: "image", SourcePath: "anything"}},
	}
	for _, drafts := range invalid {
		if _, err := s.UpdateCard(
			&cardapi.UpdateInput{FolderID: folder.ID, CardID: card.ID, Items: drafts},
		); err == nil {
			t.Fatalf("accepted invalid drafts: %+v", drafts)
		}
	}
	image, _ := mediaFixture(t, ".png")
	failed := []contentapi.Draft{
		{Kind: "image", SourcePath: image},
		{Kind: "audio", SourcePath: "missing.wav"},
	}
	for range 6 {
		failed = append(failed, contentapi.Draft{Kind: "image", SourcePath: image})
	}
	if _, err := s.UpdateCard(
		&cardapi.UpdateInput{FolderID: folder.ID, CardID: card.ID, Items: failed},
	); err == nil {
		t.Fatal("accepted missing import")
	}
	entries, _ := os.ReadDir(s.mediaDir)
	if len(entries) != 0 {
		t.Fatal("failed import left media files")
	}
	// Force a failure inside the write transaction after a successful copy.
	if err := s.db.Update(
		func(tx *bolt.Tx) error { bucket, _ := cardBucket(tx, folder.ID); return bucket.SetSequence(0) },
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCard(&cardapi.CreateInput{
		FolderID: folder.ID,
		Items: []contentapi.Draft{
			{Kind: "image", SourcePath: image},
			{Kind: "image", SourcePath: image},
			{Kind: "image", SourcePath: image},
			{Kind: "text", Text: "Answer"},
		},
	}); err == nil {
		t.Fatal("overwrote a colliding card ID")
	}
	entries, _ = os.ReadDir(s.mediaDir)
	if len(entries) != 0 {
		t.Fatal("transaction rollback left imported file")
	}
	cards, err := s.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card, otherCard}) {
		t.Fatalf("failed edit changed data: %+v, %v", cards, err)
	}
}

func TestMediaOnlyCardsStreamingAndCleanup(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(&folderapi.CreateInput{Title: "Media"})
	other, _ := s.CreateFolder(&folderapi.CreateInput{Title: "Other"})
	image, imageData := mediaFixture(t, ".png")
	audio, _ := mediaFixture(t, ".wav")
	card, err := s.CreateCard(&cardapi.CreateInput{
		FolderID: folder.ID,
		Items: []contentapi.Draft{
			{Kind: "image", SourcePath: image},
			{Kind: "audio", SourcePath: audio},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	copyCard, err := s.CreateCard(&cardapi.CreateInput{
		FolderID: folder.ID,
		Items: []contentapi.Draft{
			{Kind: "image", SourcePath: image},
			{Kind: "audio", SourcePath: audio},
		},
	})
	if err != nil || copyCard.Items[0].RelativePath == card.Items[0].RelativePath {
		t.Fatal("media was shared between cards")
	}
	url := "/media/" + folder.ID + "/" + card.ID + "/" + card.Items[0].ID
	handler := s.MediaHandler()
	for _, test := range []struct {
		method, url, byteRange string
		status                 int
		body                   []byte
	}{
		{"GET", url, "", http.StatusOK, imageData},
		{"GET", url, "bytes=2-7", http.StatusPartialContent, imageData[2:8]},
		{"HEAD", url, "", http.StatusOK, nil},
		{"GET", url, "bytes=99999-", http.StatusRequestedRangeNotSatisfiable, nil},
		{"GET", "/media/" + other.ID + "/" + card.ID + "/" + card.Items[0].ID, "", http.StatusNotFound, nil},
		{"GET", "/media/../raven.db", "", http.StatusNotFound, nil},
		{"GET", url + "/extra", "", http.StatusNotFound, nil},
		{"POST", url, "", http.StatusMethodNotAllowed, nil},
	} {
		r := httptest.NewRequest(test.method, test.url, nil)
		if test.byteRange != "" {
			r.Header.Set("Range", test.byteRange)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("%s %s: status %d", test.method, test.url, w.Code)
		}
		if test.body != nil && !bytes.Equal(w.Body.Bytes(), test.body) {
			t.Fatal("streamed bytes differ")
		}
		if test.status == http.StatusOK && w.Header().Get("Content-Type") != "image/png" {
			t.Fatal("missing media MIME type")
		}
		if test.method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD included body")
		}
	}
	if err := os.Remove(s.mediaPath(card.Items[0].RelativePath)); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
	if w.Code != http.StatusNotFound {
		t.Fatal("missing file not reported")
	}
	orphan := strings.Repeat("a", 32) + ".png"
	for _, name := range []string{orphan, ".import-abandoned", "unrelated.txt"} {
		if err := os.WriteFile(
			filepath.Join(s.mediaDir, name),
			[]byte("orphan"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
	path := s.db.Path()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, name := range []string{orphan, ".import-abandoned"} {
		if _, err := os.Stat(filepath.Join(s.mediaDir, name)); !os.IsNotExist(err) {
			t.Fatal("startup orphan cleanup failed")
		}
	}
	if _, err := os.Stat(filepath.Join(s.mediaDir, "unrelated.txt")); err != nil {
		t.Fatal("removed unmanaged file")
	}
	if _, err := os.Stat(s.mediaPath(copyCard.Items[0].RelativePath)); err != nil {
		t.Fatal("removed referenced media")
	}
	if cards, err := s.ListCards(); err != nil || len(cards) != 2 {
		t.Fatalf("missing file prevented loading other content: %v", err)
	}
}

func TestMediaTypeValidation(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(&folderapi.CreateInput{Title: "Media"})
	path := filepath.Join(t.TempDir(), "fake.png")
	if err := os.WriteFile(path, []byte("<html>not an image</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	image, _ := mediaFixture(t, ".png")
	for _, draft := range []contentapi.Draft{{Kind: "image", SourcePath: path}, {Kind: "video", SourcePath: image}, {Kind: "image", SourcePath: filepath.Dir(image)}} {
		if _, err := s.CreateCard(&cardapi.CreateInput{
			FolderID: folder.ID,
			Items: append(
				[]contentapi.Draft{draft},
				contentapi.Draft{Kind: "text", Text: "answer"},
			),
		}); err == nil {
			t.Fatalf("accepted invalid media: %+v", draft)
		}
	}
	m4a, _ := mediaFixture(t, ".m4a")
	if _, err := s.CreateCard(&cardapi.CreateInput{
		FolderID: folder.ID,
		Items: []contentapi.Draft{
			{Kind: "audio", SourcePath: m4a},
			{Kind: "text", Text: "answer"},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentReviewAndContentEdit(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(&folderapi.CreateInput{Title: "Concurrent"})
	card, err := s.CreateCard(
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("prompt", "answer")},
	)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := s.ReviewItem(
				&cardapi.ReviewInput{
					FolderID: folder.ID,
					CardID:   card.ID,
					ItemID:   card.Items[0].ID,
					Rating:   3,
				},
			); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() {
			if _, err := s.UpdateCard(&cardapi.UpdateInput{
				FolderID: folder.ID,
				CardID:   card.ID,
				Items:    itemDrafts(card),
			}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	cards, err := s.ListCards()
	if err != nil || cards[0].Items[0].Schedule.Reps != 9 || cards[0].LastReviewed == 0 ||
		cards[0].DueAt <= time.Now().UnixMilli() {
		t.Fatalf("edit lost review: %+v, %v", cards, err)
	}
}

func createLegacyDB(t *testing.T, path string, malformed bool) []byte {
	t.Helper()
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(
		`{"id":"00000000000000000001","folderId":"00000000000000000001","prompt":"Old prompt","answer":"Old answer","createdAt":1000,"dueAt":2000,"lastReviewed":1500,"stage":3}`,
	)
	if malformed {
		data = []byte(
			`{"id":"bad","folderId":"missing","prompt":"Old prompt","answer":"Old answer","createdAt":1000,"dueAt":2000}`,
		)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		root, _ := tx.CreateBucket([]byte(foldersBucket))
		_ = root.SetSequence(1)
		folder, _ := root.CreateBucket([]byte("00000000000000000001"))
		metadata, _ := json.Marshal(
			folderapi.Folder{ID: "00000000000000000001", Title: "Legacy", Tags: []string{}},
		)
		_ = folder.Put([]byte(metadataKey), metadata)
		cards, _ := folder.CreateBucket([]byte(cardsBucket))
		_ = cards.SetSequence(1)
		return cards.Put([]byte("00000000000000000001"), data)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestLegacyMigrationBackupAndIdempotence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	original := createLegacyDB(t, path, false)
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := s.ListCards()
	if err != nil || len(cards) != 1 {
		t.Fatalf("migration: %v", err)
	}
	card := cards[0]
	if card.CreatedAt != 1000 || card.DueAt <= 2000 || card.LastReviewed != 0 ||
		len(card.Items) != 2 ||
		card.Items[0].Text != "Old prompt" ||
		card.Items[1].Text != "Old answer" {
		t.Fatalf("migration lost data: %+v", card)
	}
	backups, _ := filepath.Glob(path + ".v1-*.bak")
	if len(backups) != 1 {
		t.Fatal("missing pre-migration backup")
	}
	backup, err := bolt.Open(backups[0], 0o600, &bolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	err = backup.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte(foldersBucket)).
			Bucket([]byte(card.FolderID)).
			Bucket([]byte(cardsBucket)).
			Get([]byte(card.ID))
		if !bytes.Equal(data, original) {
			t.Fatal("backup does not contain original record")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = backup.Close()
	_ = s.Close()
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	again, err := s.ListCards()
	if err != nil || !reflect.DeepEqual(cards, again) {
		t.Fatal("migration repeated or duplicated items")
	}
	backups, _ = filepath.Glob(path + ".v1-*.bak")
	if len(backups) != 1 {
		t.Fatal("made another backup after migration")
	}
	next, err := s.CreateCard(
		&cardapi.CreateInput{FolderID: card.FolderID, Items: textItems("next", "answer")},
	)
	if err != nil || next.ID != "00000000000000000002" {
		t.Fatal("lost card sequence")
	}
}

func TestFailedMigrationLeavesOriginalRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	original := createLegacyDB(t, path, true)
	if s, err := openStore(path); err == nil {
		_ = s.Close()
		t.Fatal("accepted invalid legacy ownership")
	}
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = db.View(func(tx *bolt.Tx) error {
		if tx.Bucket([]byte(metaBucket)) != nil {
			t.Fatal("failed migration committed schema version")
		}
		folder := tx.Bucket([]byte(foldersBucket)).Bucket([]byte("00000000000000000001"))
		if folder.Bucket([]byte(itemsBucket)) != nil {
			t.Fatal("failed migration left content records")
		}
		if !bytes.Equal(
			folder.Bucket([]byte(cardsBucket)).Get([]byte("00000000000000000001")),
			original,
		) {
			t.Fatal("failed migration changed original card")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSingleContentCards(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(&folderapi.CreateInput{Title: "Information"})
	card, err := s.CreateCard(
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("A fact worth remembering")},
	)
	if err != nil || len(card.Items) != 1 {
		t.Fatalf("single text card: %+v, %v", card, err)
	}
	image, _ := mediaFixture(t, ".png")
	updated, err := s.UpdateCard(&cardapi.UpdateInput{
		FolderID: folder.ID,
		CardID:   card.ID,
		Items:    []contentapi.Draft{{Kind: "image", SourcePath: image}},
	})
	if err != nil || len(updated.Items) != 1 || updated.Items[0].Kind != "image" ||
		updated.Items[0].Position != 0 {
		t.Fatalf("single media card: %+v, %v", updated, err)
	}
	if _, err := s.UpdateCard(
		&cardapi.UpdateInput{FolderID: folder.ID, CardID: card.ID, Items: nil},
	); err == nil {
		t.Fatal("accepted an empty information card")
	}
	cards, err := s.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{updated}) {
		t.Fatalf("empty edit changed saved content: %+v, %v", cards, err)
	}
}

func TestSidedContentMigration(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(fmt.Sprintf("malformed=%v", malformed), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "raven.db")
			s, err := openStore(path)
			if err != nil {
				t.Fatal(err)
			}
			folder, _ := s.CreateFolder(&folderapi.CreateInput{Title: "Existing content"})
			image, _ := mediaFixture(t, ".png")
			card, err := s.CreateCard(&cardapi.CreateInput{
				FolderID: folder.ID,
				Items: []contentapi.Draft{
					{Kind: "text", Text: "Prompt text"},
					{Kind: "image", SourcePath: image},
					{Kind: "text", Text: "Answer text"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			card, err = s.ReviewItem(
				&cardapi.ReviewInput{
					FolderID: folder.ID,
					CardID:   card.ID,
					ItemID:   card.Items[0].ID,
					Rating:   3,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			originals := map[string][]byte{}
			err = s.db.Update(func(tx *bolt.Tx) error {
				if err := tx.Bucket([]byte(metaBucket)).
					Put([]byte(schemaKey), []byte("2")); err != nil {
					return err
				}
				bucket := tx.Bucket([]byte(foldersBucket)).
					Bucket([]byte(folder.ID)).
					Bucket([]byte(itemsBucket)).
					Bucket([]byte(card.ID))
				for i, item := range card.Items {
					old := struct {
						contentapi.Item
						Side string `json:"side"`
					}{Item: item, Side: "prompt"}
					// Put the image before the text on the old prompt side.
					old.Position = 1 - i
					if i == 2 {
						old.Side = "answer"
						old.Position = 0
					}
					if malformed && i == 2 {
						old.Position = 7
					}
					data, err := json.Marshal(old)
					if err != nil {
						return err
					}
					originals[item.ID] = data
					if err := bucket.Put([]byte(item.ID), data); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = openStore(path)
			if malformed {
				if err == nil {
					_ = s.Close()
					t.Fatal("accepted invalid old item order")
				}
				db, err := bolt.Open(path, 0o600, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				err = db.View(func(tx *bolt.Tx) error {
					if string(tx.Bucket([]byte(metaBucket)).Get([]byte(schemaKey))) != "2" {
						t.Fatal("failed migration changed version")
					}
					bucket := tx.Bucket([]byte(foldersBucket)).
						Bucket([]byte(folder.ID)).
						Bucket([]byte(itemsBucket)).
						Bucket([]byte(card.ID))
					for id, data := range originals {
						if !bytes.Equal(bucket.Get([]byte(id)), data) {
							t.Fatal("failed migration changed content")
						}
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			cards, err := s.ListCards()
			if err != nil || len(cards) != 1 {
				t.Fatalf("migrated cards: %+v, %v", cards, err)
			}
			want := card
			want.Items = []contentapi.Item{card.Items[1], card.Items[0], card.Items[2]}
			for i := range want.Items {
				want.Items[i].Position = i
				want.Items[i].Schedule = cards[0].Items[i].Schedule
				want.Items[i].Reviews = cards[0].Items[i].Reviews
				if len(want.Items[i].Reviews) != 1 || !want.Items[i].Reviews[0].Automatic {
					t.Fatal("migration did not initialize Again")
				}
			}
			want.DueAt = cards[0].DueAt
			want.LastReviewed = 0
			if !reflect.DeepEqual(cards[0], want) {
				t.Fatalf("content/history lost: got %+v, want %+v", cards[0], want)
			}
			if _, err := os.Stat(s.mediaPath(want.Items[0].RelativePath)); err != nil {
				t.Fatal("migration lost media file")
			}
			err = s.db.View(func(tx *bolt.Tx) error {
				if string(tx.Bucket([]byte(metaBucket)).Get([]byte(schemaKey))) != "5" {
					t.Fatal("schema not upgraded")
				}
				bucket := tx.Bucket([]byte(foldersBucket)).
					Bucket([]byte(folder.ID)).
					Bucket([]byte(itemsBucket)).
					Bucket([]byte(card.ID))
				return bucket.ForEach(func(_, data []byte) error {
					if bytes.Contains(data, []byte(`"side"`)) {
						t.Fatal("kept side in new records")
					}
					return nil
				})
			})
			if err != nil {
				t.Fatal(err)
			}
			_ = s.Close()
			s, err = openStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			again, err := s.ListCards()
			if err != nil || !reflect.DeepEqual(cards, again) {
				t.Fatal("migration repeated")
			}
			backups, _ := filepath.Glob(path + ".v2-*.bak")
			if len(backups) != 1 {
				t.Fatal("missing or repeated v2 migration backup")
			}
		})
	}
}

func TestDeletionPreservesOtherFolders(t *testing.T) {
	s, err := openStore(filepath.Join(t.TempDir(), "raven.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	first, err := s.CreateFolder(&folderapi.CreateInput{Title: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateFolder(&folderapi.CreateInput{Title: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	create := func(folder string) cardapi.Card {
		t.Helper()
		card, err := s.CreateCard(
			&cardapi.CreateInput{FolderID: folder, Items: textItems("Remember")},
		)
		if err != nil {
			t.Fatal(err)
		}
		return card
	}
	removed := create(first.ID)
	create(first.ID)
	survivor := create(second.ID)
	if err := s.DeleteCard(first.ID, removed.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		if tx.Bucket([]byte(foldersBucket)).
			Bucket([]byte(first.ID)).
			Bucket([]byte(itemsBucket)).
			Bucket([]byte(removed.ID)) !=
			nil {
			t.Error("deleted card still has item records")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cards, err := s.ListCards()
	if err != nil || len(cards) != 2 {
		t.Fatalf("after card deletion: %v, %v", cards, err)
	}
	if err := s.DeleteCard(first.ID, removed.ID); err == nil {
		t.Fatal("expected missing card error")
	}
	if err := s.DeleteFolder(first.ID); err != nil {
		t.Fatal(err)
	}
	cards, err = s.ListCards()
	if err != nil || len(cards) != 1 || cards[0].FolderID != survivor.FolderID ||
		cards[0].ID != survivor.ID {
		t.Fatalf("after folder deletion: %v, %v", cards, err)
	}
	folders, err := s.ListFolders()
	if err != nil || len(folders) != 1 || folders[0].ID != second.ID {
		t.Fatalf("remaining folders: %v, %v", folders, err)
	}
	if err := s.DeleteFolder(first.ID); err == nil {
		t.Fatal("expected missing folder error")
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := openStore(filepath.Join(t.TempDir(), "raven.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestEmptyDatabaseAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven", "raven.db")
	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	folders, err := store.ListFolders()
	if err != nil || folders == nil || len(folders) != 0 {
		t.Fatalf("empty database: folders=%v, error=%v", folders, err)
	}
	data, err := json.Marshal(folders)
	if err != nil || string(data) != "[]" {
		t.Fatalf("expected JSON [], got %s, error=%v", data, err)
	}
	if runtime.GOOS != "windows" {
		for name, want := range map[string]os.FileMode{filepath.Dir(path): 0o700, path: 0o600} {
			info, err := os.Stat(name)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != want {
				t.Errorf("permissions for %s: got %o, want %o", name, info.Mode().Perm(), want)
			}
		}
	}
}

func TestFoldersPersistInCreationOrderWithCardsBuckets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	want := make([]folderapi.Folder, 0)
	for i := range 12 {
		folder, err := store.CreateFolder(&folderapi.CreateInput{
			Title: fmt.Sprintf("  Ideas %d  ", i), Description: "  Things to explore 新しい  ",
			Tags: []string{" ideas ", "", "Personal", "IDEAS", "personal", " 新しい "},
		})
		if err != nil {
			t.Fatal(err)
		}
		if folder.ID != fmt.Sprintf("%020d", i+1) || folder.Title != fmt.Sprintf("Ideas %d", i) ||
			folder.Description != "Things to explore 新しい" || !reflect.DeepEqual(folder.Tags, []string{"ideas", "Personal", "新しい"}) {
			t.Fatalf("unexpected folder: %+v", folder)
		}
		want = append(want, folder)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.ListFolders()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("reopened folders: got=%+v, want=%+v, error=%v", got, want, err)
	}
	err = store.db.View(func(tx *bolt.Tx) error {
		root := tx.Bucket([]byte(foldersBucket))
		for _, folder := range want {
			bucket := root.Bucket([]byte(folder.ID))
			if bucket == nil || bucket.Get([]byte(metadataKey)) == nil ||
				bucket.Bucket([]byte(cardsBucket)) == nil {
				return fmt.Errorf("folder %s missing metadata or cards bucket", folder.ID)
			}
			if key, _ := bucket.Bucket([]byte(cardsBucket)).Cursor().First(); key != nil {
				return fmt.Errorf("folder %s cards bucket is not empty", folder.ID)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := store.CreateFolder(&folderapi.CreateInput{Title: "After restart"})
	if err != nil || next.ID != fmt.Sprintf("%020d", len(want)+1) || next.Tags == nil {
		t.Fatalf("sequence/tags after reopen: %+v, error=%v", next, err)
	}
}

func TestValidationDoesNotWrite(t *testing.T) {
	store := openTestStore(t)
	for _, title := range []string{"", " \n\t ", "\u3000"} {
		if _, err := store.CreateFolder(&folderapi.CreateInput{Title: title}); err == nil {
			t.Errorf("accepted empty title %q", title)
		}
	}
	folders, err := store.ListFolders()
	if err != nil || len(folders) != 0 {
		t.Fatalf("validation wrote folders: %v, error=%v", folders, err)
	}
	folder, err := store.CreateFolder(&folderapi.CreateInput{Title: "Valid"})
	if err != nil || folder.ID != "00000000000000000001" || folder.Tags == nil {
		t.Fatalf("invalid sequence or null tags: %+v, error=%v", folder, err)
	}
}

func TestConcurrentFolderCreation(t *testing.T) {
	store := openTestStore(t)
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			if _, err := store.CreateFolder(
				&folderapi.CreateInput{Title: fmt.Sprintf("folderapi.Folder %d", i)},
			); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	folders, err := store.ListFolders()
	if err != nil || len(folders) != 32 {
		t.Fatalf("concurrent folders: count=%d, error=%v", len(folders), err)
	}
	for i, folder := range folders {
		if folder.ID != fmt.Sprintf("%020d", i+1) {
			t.Errorf("unexpected ID at index %d: %s", i, folder.ID)
		}
	}
}

func TestFailedCreationRollsBackSequenceAndPreservesExistingFolder(t *testing.T) {
	store := openTestStore(t)
	original, err := store.CreateFolder(&folderapi.CreateInput{Title: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	// Force a collision after NextSequence so the failed transaction must roll it back.
	if err := store.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(foldersBucket)).SetSequence(0)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateFolder(
		&folderapi.CreateInput{Title: "Must not overwrite"},
	); !errors.Is(
		err,
		bolt.ErrBucketExists,
	) {
		t.Fatalf("expected bucket collision, got %v", err)
	}
	if err := store.db.View(func(tx *bolt.Tx) error {
		if sequence := tx.Bucket([]byte(foldersBucket)).Sequence(); sequence != 0 {
			return fmt.Errorf("failed transaction advanced sequence to %d", sequence)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	folders, err := store.ListFolders()
	if err != nil || !reflect.DeepEqual(folders, []folderapi.Folder{original}) {
		t.Fatalf("failed transaction changed folders: %+v, error=%v", folders, err)
	}
}

func TestDatabaseLockTimesOut(t *testing.T) {
	store := openTestStore(t)
	start := time.Now()
	other, err := openStore(store.db.Path())
	if other != nil {
		_ = other.Close()
	}
	if !errors.Is(err, bolt.ErrTimeout) || !strings.Contains(err.Error(), store.db.Path()) {
		t.Fatalf("expected lock timeout with path, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond || elapsed > 5*time.Second {
		t.Errorf("unexpected lock timeout duration: %v", elapsed)
	}
}

func TestUnreadableDatabaseIsNotReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	original := bytes.Repeat([]byte("not a database"), 1024)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := openStore(path)
	if store != nil {
		_ = store.Close()
	}
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("expected open error with path, got %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("corrupt database was changed, error=%v", err)
	}
}

func TestReadFailureDoesNotReturnPartialFolders(t *testing.T) {
	store := openTestStore(t)
	for range 2 {
		if _, err := store.CreateFolder(&folderapi.CreateInput{Title: "Ideas"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(foldersBucket)).
			Bucket([]byte("00000000000000000002")).
			Put([]byte(metadataKey), []byte("{broken"))
	}); err != nil {
		t.Fatal(err)
	}
	if folders, err := store.ListFolders(); err == nil || folders != nil {
		t.Fatalf("expected error without partial results: %+v, error=%v", folders, err)
	}
}

func TestClosedStoreReturnsErrors(t *testing.T) {
	store := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListFolders(); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("expected closed database error, got %v", err)
	}
	if _, err := store.CreateFolder(&folderapi.CreateInput{Title: "Ideas"}); !errors.Is(
		err,
		bolt.ErrDatabaseNotOpen,
	) {
		t.Fatalf("expected closed database error, got %v", err)
	}
}

func TestItemReviewValidationAndScheduleEdits(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(&folderapi.CreateInput{Title: "Learning"})
	card, err := s.CreateCard(
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("one", "two")},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []struct {
		item   string
		rating int
	}{{card.Items[0].ID, 0}, {card.Items[0].ID, 5}, {"missing", 3}, {card.Items[1].ID, -1}} {
		if _, err := s.ReviewItem(
			&cardapi.ReviewInput{
				FolderID: folder.ID,
				CardID:   card.ID,
				ItemID:   invalid.item,
				Rating:   invalid.rating,
			},
		); err == nil {
			t.Fatal("accepted invalid review")
		}
	}
	cards, _ := s.ListCards()
	if !reflect.DeepEqual(cards[0], card) {
		t.Fatal("invalid review mutated data")
	}
	// An invalid FSRS time must roll back both the schedule and its review log.
	if _, err := s.reviewItemAt(
		&cardapi.ReviewInput{
			FolderID: folder.ID,
			CardID:   card.ID,
			ItemID:   card.Items[0].ID,
			Rating:   3,
		},
		card.Items[0].Schedule.LastReview.Add(-time.Second),
	); err == nil {
		t.Fatal("accepted review before last review")
	}
	cards, _ = s.ListCards()
	if !reflect.DeepEqual(cards[0], card) {
		t.Fatal("failed FSRS operation mutated data")
	}
	updated, err := s.UpdateCard(
		&cardapi.UpdateInput{FolderID: folder.ID, CardID: card.ID, Items: []contentapi.Draft{
			{ID: card.Items[1].ID, Kind: "text", Text: "edited"}, {Kind: "text", Text: "new"},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updated.Items[0].Schedule, card.Items[1].Schedule) ||
		!reflect.DeepEqual(updated.Items[0].Reviews, card.Items[1].Reviews) {
		t.Fatal("edit reset schedule")
	}
	if updated.Items[1].Schedule.Reps != 1 || !updated.Items[1].Reviews[0].Automatic {
		t.Fatal("new item did not start Again")
	}
	if _, err := s.ReviewItem(
		&cardapi.ReviewInput{
			FolderID: folder.ID,
			CardID:   card.ID,
			ItemID:   card.Items[0].ID,
			Rating:   3,
		},
	); err == nil {
		t.Fatal("removed item remains reviewable")
	}
}

func TestNotificationAcknowledgmentPersistsAndRejectsStaleSchedules(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(&folderapi.CreateInput{Title: "Reminders"})
	card, _ := s.CreateCard(
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("one", "two")},
	)
	first := card.Items[0]
	occurrence := reviewapi.DueOccurrence{
		FolderID: folder.ID,
		CardID:   card.ID,
		ItemID:   first.ID,
		Due:      first.Schedule.Due.UnixMilli(),
	}
	if err := s.MarkNotified([]reviewapi.DueOccurrence{occurrence}); err != nil {
		t.Fatal(err)
	}
	card, err := s.ReviewItem(
		&cardapi.ReviewInput{FolderID: folder.ID, CardID: card.ID, ItemID: first.ID, Rating: 4},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkNotified([]reviewapi.DueOccurrence{occurrence}); err != nil {
		t.Fatal(err)
	}
	cards, _ := s.ListCards()
	if cards[0].Items[0].LastNotifiedDue != occurrence.Due ||
		cards[0].Items[1].LastNotifiedDue != 0 ||
		cards[0].Items[0].LastNotifiedDue == card.Items[0].Schedule.Due.UnixMilli() {
		t.Fatal("stale acknowledgment suppressed new review")
	}
	claimed, err := s.ClaimNotificationPrompt()
	if err != nil || !claimed {
		t.Fatal("first authorization not claimed")
	}
	path := s.db.Path()
	_ = s.Close()
	reopened, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if claimed, err := reopened.ClaimNotificationPrompt(); err != nil || claimed {
		t.Fatal("authorization prompted twice")
	}
	persisted, err := reopened.ListCards()
	if err != nil || !reflect.DeepEqual(persisted, cards) {
		t.Fatal("notification metadata lost after restart")
	}
}

func TestV3MigrationInitializesAllItemsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	folder, _ := s.CreateFolder(&folderapi.CreateInput{Title: "V3"})
	image, _ := mediaFixture(t, ".png")
	card, err := s.CreateCard(&cardapi.CreateInput{
		FolderID: folder.ID,
		Items: []contentapi.Draft{
			{Kind: "text", Text: "text"},
			{Kind: "image", SourcePath: image},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket([]byte(metaBucket)).Put([]byte(schemaKey), []byte("3")); err != nil {
			return err
		}
		bucket := tx.Bucket([]byte(foldersBucket)).
			Bucket([]byte(folder.ID)).
			Bucket([]byte(itemsBucket)).
			Bucket([]byte(card.ID))
		for _, item := range card.Items {
			data, _ := json.Marshal(item)
			var old map[string]json.RawMessage
			_ = json.Unmarshal(data, &old)
			delete(old, "schedule")
			delete(old, "reviews")
			delete(old, "lastNotifiedDue")
			data, _ = json.Marshal(old)
			if err := bucket.Put([]byte(item.ID), data); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := s.ListCards()
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range cards[0].Items {
		if item.ID != card.Items[i].ID || item.RelativePath != card.Items[i].RelativePath ||
			item.Schedule.Reps != 1 ||
			!item.Reviews[0].Automatic {
			t.Fatal("migration lost content or failed to initialize Again")
		}
	}
	backups, _ := filepath.Glob(path + ".v3-*.bak")
	if len(backups) != 1 {
		t.Fatal("missing migration backup")
	}
	_ = s.Close()
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	again, err := s.ListCards()
	if err != nil || !reflect.DeepEqual(cards, again) {
		t.Fatal("FSRS migration repeated")
	}
}

func TestAutostartMarkerPersistsWithoutChangingContentOrSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	folder, err := s.CreateFolder(
		&folderapi.CreateInput{Title: "Existing folder", Tags: []string{"retained"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err := s.CreateCard(
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("Existing memory")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if first, err := s.ClaimAutostartInitialization(); err != nil || !first {
		t.Fatalf("first claim=%v error=%v", first, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if first, err := s.ClaimAutostartInitialization(); err != nil || first {
		t.Fatalf("reopened claim=%v error=%v", first, err)
	}
	folders, err := s.ListFolders()
	if err != nil || !reflect.DeepEqual(folders, []folderapi.Folder{folder}) {
		t.Fatalf("folders changed: %+v, %v", folders, err)
	}
	cards, err := s.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card}) {
		t.Fatalf("cards changed: %+v, %v", cards, err)
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		if string(tx.Bucket([]byte(metaBucket)).Get([]byte(schemaKey))) != "5" {
			t.Fatal("settings changed schema version")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAutostartMarkerClaimIsAtomic(t *testing.T) {
	s := openTestStore(t)
	var claims atomic.Int32
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			first, err := s.ClaimAutostartInitialization()
			if err != nil {
				t.Error(err)
			}
			if first {
				claims.Add(1)
			}
		})
	}
	wg.Wait()
	if claims.Load() != 1 {
		t.Fatalf("claimed initialization %d times", claims.Load())
	}
}

func TestAutostartMarkerFailsOnClosedDatabase(t *testing.T) {
	s := openTestStore(t)
	_ = s.Close()
	if first, err := s.ClaimAutostartInitialization(); err == nil || first {
		t.Fatalf("closed database claim=%v error=%v", first, err)
	}
}

func openStore(path string) (*Store, error) {
	return Open(
		path,
		&Dependencies{
			Batch:     cffbatch.New(),
			Scheduler: scheduling.New(&configurationFixture().Library),
			Logger:    nil,
			Settings:  *storeSettingsFixture(),
		},
	)
}

func nativeSchedule(v reviewapi.Schedule) fsrs.Card {
	return fsrs.Card{
		Due:            v.Due,
		Stability:      v.Stability,
		Difficulty:     v.Difficulty,
		ScheduledDays:  v.ScheduledDays,
		Reps:           v.Reps,
		Lapses:         v.Lapses,
		State:          fsrs.State(v.State),
		LastReview:     v.LastReview,
		RemainingSteps: v.RemainingSteps,
	}
}

func TestV4MigrationPreservesSchedulesHistoryMediaAndMarkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	storage, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := storage.CreateFolder(&folderapi.CreateInput{Title: "Existing"})
	if err != nil {
		t.Fatal(err)
	}
	media, contents := mediaFixture(t, ".png")
	card, err := storage.CreateCard(
		&cardapi.CreateInput{FolderID: folder.ID, Items: []contentapi.Draft{
			{Kind: "text", Text: "Keep this schedule"}, {Kind: "image", SourcePath: media},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err = storage.ReviewItem(
		&cardapi.ReviewInput{
			FolderID: folder.ID,
			CardID:   card.ID,
			ItemID:   card.Items[0].ID,
			Rating:   3,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = storage.MarkNotified(
		[]reviewapi.DueOccurrence{
			{
				FolderID: folder.ID,
				CardID:   card.ID,
				ItemID:   card.Items[0].ID,
				Due:      card.Items[0].Schedule.Due.UnixMilli(),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := storage.ListCards()
	if err != nil {
		t.Fatal(err)
	}
	want := cards[0]
	claimed, err := storage.ClaimAutostartInitialization()
	if err != nil || !claimed {
		t.Fatalf("claim startup: %v %v", claimed, err)
	}
	claimed, err = storage.ClaimNotificationPrompt()
	if err != nil || !claimed {
		t.Fatalf("claim prompt: %v %v", claimed, err)
	}
	err = storage.db.Update(func(tx *bolt.Tx) error {
		folderBucket := tx.Bucket([]byte(foldersBucket)).Bucket([]byte(folder.ID))
		cardsBucket := folderBucket.Bucket([]byte(cardsBucket))
		if err := cardsBucket.Put(
			[]byte(card.ID),
			v4JSON(t, cardsBucket.Get([]byte(card.ID))),
		); err != nil {
			return err
		}
		items := folderBucket.Bucket([]byte(itemsBucket)).Bucket([]byte(card.ID))
		for _, item := range want.Items {
			if err := items.Put(
				[]byte(item.ID),
				v4JSON(t, items.Get([]byte(item.ID))),
			); err != nil {
				return err
			}
		}
		return tx.Bucket([]byte(metaBucket)).Put([]byte(schemaKey), []byte("4"))
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	for attempt := range 2 {
		reopened, err := openStore(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := reopened.ListCards()
		if err != nil || !reflect.DeepEqual(got, []cardapi.Card{want}) {
			t.Fatalf("lost records: %+v %v", got, err)
		}
		if claimed, err := reopened.ClaimAutostartInitialization(); err != nil || claimed {
			t.Fatalf("lost startup marker: %v %v", claimed, err)
		}
		if claimed, err := reopened.ClaimNotificationPrompt(); err != nil || claimed {
			t.Fatalf("lost prompt marker: %v %v", claimed, err)
		}
		data, err := os.ReadFile(reopened.mediaPath(want.Items[1].RelativePath))
		if err != nil || !bytes.Equal(data, contents) {
			t.Fatalf("lost media: %v", err)
		}
		err = reopened.db.View(func(tx *bolt.Tx) error {
			if string(tx.Bucket([]byte(metaBucket)).Get([]byte(schemaKey))) != "5" {
				t.Fatal("schema not upgraded")
			}
			data := tx.Bucket([]byte(foldersBucket)).
				Bucket([]byte(folder.ID)).
				Bucket([]byte(cardsBucket)).
				Get([]byte(card.ID))
			if !bytes.Contains(data, []byte(`"folder_id"`)) ||
				bytes.Contains(data, []byte(`"folderId"`)) {
				t.Fatalf("wrong persisted keys: %s", data)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
		backups, err := filepath.Glob(path + ".v4-*.bak")
		if err != nil || len(backups) != 1 {
			t.Fatalf("attempt %d backups %v error %v", attempt, backups, err)
		}
	}
}

func v4JSON(t *testing.T, data []byte) []byte {
	t.Helper()
	// These are the historical schema-4 field names, independent of Go struct tags.
	replacer := strings.NewReplacer(
		`"folder_id":`, `"folderId":`, `"card_id":`, `"cardId":`,
		`"created_at":`, `"createdAt":`, `"due_at":`, `"dueAt":`,
		`"last_reviewed":`, `"lastReviewed":`, `"original_name":`, `"originalName":`,
		`"mime_type":`, `"mimeType":`, `"relative_path":`, `"relativePath":`,
		`"byte_size":`, `"byteSize":`, `"last_notified_due":`, `"lastNotifiedDue":`,
		`"due":`, `"Due":`, `"last_review":`, `"LastReview":`,
		`"stability":`, `"Stability":`, `"difficulty":`, `"Difficulty":`,
		`"scheduled_days":`, `"ScheduledDays":`, `"reps":`, `"Reps":`,
		`"lapses":`, `"Lapses":`, `"remaining_steps":`, `"RemainingSteps":`,
		`"state":`, `"State":`, `"review":`, `"Review":`, `"rating":`, `"Rating":`,
	)
	result := []byte(replacer.Replace(string(data)))
	if !json.Valid(result) {
		t.Fatal("invalid legacy fixture")
	}
	return result
}

func TestV4MigrationFailureRollsBackOriginalRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	storage, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := storage.CreateFolder(&folderapi.CreateInput{Title: "Existing"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := storage.CreateCard(
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("Keep me")},
	)
	if err != nil {
		t.Fatal(err)
	}
	var original []byte
	err = storage.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(foldersBucket)).
			Bucket([]byte(folder.ID)).
			Bucket([]byte(cardsBucket))
		original = v4JSON(t, bucket.Get([]byte(card.ID)))
		if err := bucket.Put([]byte(card.ID), original); err != nil {
			return err
		}
		items := tx.Bucket([]byte(foldersBucket)).
			Bucket([]byte(folder.ID)).
			Bucket([]byte(itemsBucket)).
			Bucket([]byte(card.ID))
		if err := items.Put([]byte(card.Items[0].ID), []byte(`{"invalid":`)); err != nil {
			return err
		}
		return tx.Bucket([]byte(metaBucket)).Put([]byte(schemaKey), []byte("4"))
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	if failed, err := openStore(path); err == nil {
		failed.Close()
		t.Fatal("accepted corrupted legacy record")
	}
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = db.View(func(tx *bolt.Tx) error {
		if string(tx.Bucket([]byte(metaBucket)).Get([]byte(schemaKey))) != "4" {
			t.Fatal("failed migration advanced schema")
		}
		data := tx.Bucket([]byte(foldersBucket)).
			Bucket([]byte(folder.ID)).
			Bucket([]byte(cardsBucket)).
			Get([]byte(card.ID))
		if !bytes.Equal(data, original) {
			t.Fatal("failed migration rewrote original records")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecordDecoderRejectsMalformedAndAmbiguousData(t *testing.T) {
	for _, data := range []string{"", "{", "[", `{"folderId":"one","folder_id":"two"}`, `{"items":[{"Due":1,"due":2}]}`} {
		var target map[string]any
		if err := decodeRecord([]byte(data), &target); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
}

func TestMediaCopyPreservesReadAndCloseFailures(t *testing.T) {
	source, err := os.CreateTemp(t.TempDir(), "source-*")
	if err != nil {
		t.Fatal(err)
	}
	if err = source.Close(); err != nil {
		t.Fatal(err)
	}
	target, err := os.CreateTemp(t.TempDir(), "target-*")
	if err != nil {
		t.Fatal(err)
	}
	if err = target.Close(); err != nil {
		t.Fatal(err)
	}
	size, err := copyMediaFile(target, source)
	if size != 0 || !errors.Is(err, os.ErrClosed) {
		t.Fatalf("expected closed-file failure without copied bytes, got %d, %v", size, err)
	}
	failures := multierr.Errors(errors.Unwrap(err))
	if len(failures) != 2 {
		t.Fatalf("expected read and close failures, got %v", failures)
	}
	for index, path := range []string{source.Name(), target.Name()} {
		var failure *os.PathError
		if !errors.As(failures[index], &failure) || failure.Path != path ||
			!errors.Is(failure, os.ErrClosed) {
			t.Fatalf("expected failure for %q, got %v", path, failures[index])
		}
	}
}

func TestMediaStagingFailureClosesAndRemovesTemporaryFiles(t *testing.T) {
	store := openTestStore(t)
	path, content := mediaFixture(t, ".png")
	source, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := source.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	item := contentapi.Item{ID: strings.Repeat("a", 32), Kind: "image"}
	if err = storeSettingsFixture().inspectImportedMedia(source, path, &item); err != nil {
		t.Fatal(err)
	}
	if item.ByteSize != int64(len(content)) || item.MIMEType != "image/png" {
		t.Fatalf("unexpected imported metadata: %+v", item)
	}
	item.ByteSize++
	if _, err = store.persistMedia(
		source,
		&item,
	); !errors.Is(
		err,
		errSelectedFileChangedDuringImportTryAgain,
	) {
		t.Fatalf("expected changed-file rejection, got %v", err)
	}
	entries, err := os.ReadDir(store.mediaDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed import left files: %v, %v", entries, err)
	}

	closed, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = closed.Close(); err != nil {
		t.Fatal(err)
	}
	target, err := os.CreateTemp(t.TempDir(), "copy-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = copyMediaFile(target, closed); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("expected read failure, got %v", err)
	}
	if err = target.Close(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("copy failure left destination open: %v", err)
	}
	if err = storeSettingsFixture().inspectImportedMedia(
		closed,
		path,
		&item,
	); !errors.Is(
		err,
		os.ErrClosed,
	) {
		t.Fatalf("expected stat failure, got %v", err)
	}
	if _, err = store.commitMedia(
		filepath.Join(t.TempDir(), "missing"),
		store.mediaPath(item.RelativePath),
	); err == nil {
		t.Fatal("missing staged file committed")
	}
}

func TestStoredContentIsValidatedWhenRead(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*contentapi.Item)
	}{
		{"item ownership", func(item *contentapi.Item) { item.ID = strings.Repeat("b", 32) }},
		{"card ownership", func(item *contentapi.Item) { item.CardID = "other" }},
		{"folder ownership", func(item *contentapi.Item) { item.FolderID = "other" }},
		{"content structure", func(item *contentapi.Item) { item.Text = "" }},
		{"missing due", func(item *contentapi.Item) { item.Schedule.Due = time.Time{} }},
		{
			"missing review time",
			func(item *contentapi.Item) { item.Schedule.LastReview = time.Time{} },
		},
		{"missing history", func(item *contentapi.Item) { item.Reviews = nil }},
		{"invalid schedule", func(item *contentapi.Item) { item.Schedule.State = -1 }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			store := openTestStore(t)
			folder, err := store.CreateFolder(&folderapi.CreateInput{Title: "Stored validation"})
			if err != nil {
				t.Fatal(err)
			}
			card, err := store.CreateCard(
				&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("content")},
			)
			if err != nil {
				t.Fatal(err)
			}
			item := card.Items[0]
			test.mutate(&item)
			data, err := json.Marshal(item)
			if err != nil {
				t.Fatal(err)
			}
			err = store.db.Update(func(tx *bolt.Tx) error {
				return tx.Bucket([]byte(foldersBucket)).Bucket([]byte(folder.ID)).
					Bucket([]byte(itemsBucket)).
					Bucket([]byte(card.ID)).Put([]byte(card.Items[0].ID), data)
			})
			if err != nil {
				t.Fatal(err)
			}
			if cards, readErr := store.ListCards(); readErr == nil || cards != nil {
				t.Fatalf("invalid stored content returned: %+v, %v", cards, readErr)
			}
		})
	}
}

func TestCardReadFailureDoesNotReturnPartialResults(t *testing.T) {
	store := openTestStore(t)
	folder, err := store.CreateFolder(&folderapi.CreateInput{Title: "Partial reads"})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		if _, err = store.CreateCard(
			&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("valid")},
		); err != nil {
			t.Fatal(err)
		}
	}
	cards, err := store.ListCards()
	if err != nil || len(cards) != 2 {
		t.Fatalf("cards=%v error=%v", cards, err)
	}
	err = store.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(foldersBucket)).
			Bucket([]byte(folder.ID)).
			Bucket([]byte(cardsBucket)).
			Put([]byte(cards[1].ID), []byte("invalid"))
	})
	if err != nil {
		t.Fatal(err)
	}
	if cards, readErr := store.ListCards(); readErr == nil || cards != nil {
		t.Fatalf("returned partial cards after corruption: %v, %v", cards, readErr)
	}
}

func TestRecordEncoderRejectsNonFiniteScheduleValues(t *testing.T) {
	item := contentapi.Item{Schedule: reviewapi.Schedule{Stability: math.NaN()}}
	if _, err := encodeRecord(item); err == nil {
		t.Fatal("encoded non-finite schedule")
	}
}

func configurationFixture() *configuration.Settings {
	settings, err := (configyaml.Loader{}).Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func storeSettingsFixture() *Settings {
	settings := configurationFixture()
	return &Settings{
		Rules:            settings.Library,
		MediaRoutePrefix: settings.MediaRoutePrefix,
		LogErrorKey:      settings.LogErrorKey,
	}
}

func TestConfiguredContentAndMediaRouteSurvivePersistence(t *testing.T) {
	settings := storeSettingsFixture()
	settings.Rules.TextKind = "plain"
	settings.MediaRoutePrefix = "/saved/"
	path := filepath.Join(t.TempDir(), "raven.db")
	deps := &Dependencies{
		Batch:     cffbatch.New(),
		Scheduler: scheduling.New(&settings.Rules),
		Settings:  *settings,
	}
	store, err := Open(path, deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	folder, err := store.CreateFolder(&folderapi.CreateInput{Title: "Configured"})
	if err != nil {
		t.Fatal(err)
	}
	image, contents := mediaFixture(t, ".png")
	card, err := store.CreateCard(
		&cardapi.CreateInput{FolderID: folder.ID, Items: []contentapi.Draft{
			{Kind: "plain", Text: "Remember"}, {Kind: "image", SourcePath: image},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	cards, err := reopened.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card}) {
		t.Fatal(cards, err)
	}
	route := "/saved/" + folder.ID + "/" + card.ID + "/" + card.Items[1].ID
	response := httptest.NewRecorder()
	reopened.MediaHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), contents) {
		t.Fatalf("configured media route failed: %d %q", response.Code, response.Body.String())
	}
}
