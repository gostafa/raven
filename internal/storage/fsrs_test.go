package storage

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestItemReviewValidationAndScheduleEdits(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(CreateFolderInput{Title: "Learning"})
	card, err := s.CreateCard(CreateCardInput{FolderID: folder.ID, Items: textItems("one", "two")})
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []struct {
		item   string
		rating int
	}{{card.Items[0].ID, 0}, {card.Items[0].ID, 5}, {"missing", 3}, {card.Items[1].ID, -1}} {
		if _, err := s.ReviewItem(folder.ID, card.ID, invalid.item, invalid.rating); err == nil {
			t.Fatal("accepted invalid review")
		}
	}
	cards, _ := s.ListCards()
	if !reflect.DeepEqual(cards[0], card) {
		t.Fatal("invalid review mutated data")
	}
	// An invalid FSRS time must roll back both the schedule and its review log.
	if _, err := s.reviewItemAt(folder.ID, card.ID, card.Items[0].ID, 3, card.Items[0].Schedule.LastReview.Add(-time.Second)); err == nil {
		t.Fatal("accepted review before last review")
	}
	cards, _ = s.ListCards()
	if !reflect.DeepEqual(cards[0], card) {
		t.Fatal("failed FSRS operation mutated data")
	}
	updated, err := s.UpdateCard(UpdateCardInput{FolderID: folder.ID, CardID: card.ID, Items: []ItemDraft{
		{ID: card.Items[1].ID, Kind: "text", Text: "edited"}, {Kind: "text", Text: "new"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updated.Items[0].Schedule, card.Items[1].Schedule) || !reflect.DeepEqual(updated.Items[0].Reviews, card.Items[1].Reviews) {
		t.Fatal("edit reset schedule")
	}
	if updated.Items[1].Schedule.Reps != 1 || !updated.Items[1].Reviews[0].Automatic {
		t.Fatal("new item did not start Again")
	}
	if _, err := s.ReviewItem(folder.ID, card.ID, card.Items[0].ID, 3); err == nil {
		t.Fatal("removed item remains reviewable")
	}
}

func TestNotificationAcknowledgmentPersistsAndRejectsStaleSchedules(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(CreateFolderInput{Title: "Reminders"})
	card, _ := s.CreateCard(CreateCardInput{FolderID: folder.ID, Items: textItems("one", "two")})
	first := card.Items[0]
	occurrence := DueOccurrence{folder.ID, card.ID, first.ID, first.Schedule.Due.UnixMilli()}
	if err := s.MarkNotified([]DueOccurrence{occurrence}); err != nil {
		t.Fatal(err)
	}
	card, err := s.ReviewItem(folder.ID, card.ID, first.ID, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkNotified([]DueOccurrence{occurrence}); err != nil {
		t.Fatal(err)
	}
	cards, _ := s.ListCards()
	if cards[0].Items[0].LastNotifiedDue != occurrence.Due || cards[0].Items[1].LastNotifiedDue != 0 || cards[0].Items[0].LastNotifiedDue == card.Items[0].Schedule.Due.UnixMilli() {
		t.Fatal("stale acknowledgment suppressed new review")
	}
	claimed, err := s.ClaimNotificationPrompt()
	if err != nil || !claimed {
		t.Fatal("first authorization not claimed")
	}
	path := s.db.Path()
	_ = s.Close()
	reopened, err := Open(path)
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
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	folder, _ := s.CreateFolder(CreateFolderInput{Title: "V3"})
	image, _ := mediaFixture(t, ".png")
	card, err := s.CreateCard(CreateCardInput{FolderID: folder.ID, Items: []ItemDraft{{Kind: "text", Text: "text"}, {Kind: "image", SourcePath: image}}})
	if err != nil {
		t.Fatal(err)
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(metaBucket).Put(schemaKey, []byte("3")); err != nil {
			return err
		}
		bucket := tx.Bucket(foldersBucket).Bucket([]byte(folder.ID)).Bucket(itemsBucket).Bucket([]byte(card.ID))
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
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := s.ListCards()
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range cards[0].Items {
		if item.ID != card.Items[i].ID || item.RelativePath != card.Items[i].RelativePath || item.Schedule.Reps != 1 || !item.Reviews[0].Automatic {
			t.Fatal("migration lost content or failed to initialize Again")
		}
	}
	backups, _ := filepath.Glob(path + ".v3-*.bak")
	if len(backups) != 1 {
		t.Fatal("missing migration backup")
	}
	_ = s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	again, err := s.ListCards()
	if err != nil || !reflect.DeepEqual(cards, again) {
		t.Fatal("FSRS migration repeated")
	}
}
