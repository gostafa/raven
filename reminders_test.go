package main

import (
	"errors"
	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"path/filepath"
	"raven/internal/storage"
	"testing"
	"time"
)

func TestReminderSnapshotGroupsDueItemsAndFindsNextDeadline(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	item := func(id string, due time.Time, acknowledged bool) storage.ContentItem {
		value := storage.ContentItem{ID: id, Schedule: fsrs.NewCard(due)}
		if acknowledged {
			value.LastNotifiedDue = due.UnixMilli()
		}
		return value
	}
	cards := []storage.Card{{ID: "card", FolderID: "folder", Items: []storage.ContentItem{
		item("overdue", now.Add(-time.Hour), false), item("due", now, false), item("notified", now, true), item("future", now.Add(2*time.Second), false),
	}}}
	pending, delay := reminderSnapshot(cards, now)
	if len(pending) != 2 || delay != 2*time.Second || pending[0].ItemID != "overdue" || pending[1].ItemID != "due" {
		t.Fatalf("pending=%+v delay=%v", pending, delay)
	}
	// Simulate resume: future work is now overdue and gets reconciled immediately.
	pending, delay = reminderSnapshot(cards, now.Add(time.Hour))
	if len(pending) != 3 || delay != 30*time.Second {
		t.Fatal("resume missed due items")
	}
}

type fakeReminderNotifier struct {
	allowed bool
	fail    bool
	sent    []notifications.NotificationOptions
}

func (n *fakeReminderNotifier) CheckNotificationAuthorization() (bool, error) { return n.allowed, nil }
func (n *fakeReminderNotifier) SendNotification(options notifications.NotificationOptions) error {
	if n.fail {
		return errors.New("delivery failed")
	}
	n.sent = append(n.sent, options)
	return nil
}

func TestReminderDeliveryPermissionFailureAndRestartDeduplication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	folder, _ := store.CreateFolder(storage.CreateFolderInput{Title: "Due"})
	card, err := store.CreateCard(storage.CreateCardInput{FolderID: folder.ID, Items: []storage.ItemDraft{{Kind: "text", Text: "one"}, {Kind: "text", Text: "two"}}})
	if err != nil {
		t.Fatal(err)
	}
	now := card.Items[0].Schedule.Due.Add(time.Second)
	cards, _ := store.ListCards()
	pending, _ := reminderSnapshot(cards, now)
	notifier := &fakeReminderNotifier{}
	if err := deliverReminders(store, notifier, pending); err != nil {
		t.Fatal(err)
	}
	cards, _ = store.ListCards()
	pending, _ = reminderSnapshot(cards, now)
	if len(notifier.sent) != 0 || len(pending) != 2 {
		t.Fatal("denial sent an alert or acknowledged work")
	}
	notifier.allowed = true
	notifier.fail = true
	if err := deliverReminders(store, notifier, pending); err == nil {
		t.Fatal("delivery failure hidden")
	}
	cards, _ = store.ListCards()
	pending, _ = reminderSnapshot(cards, now)
	if len(pending) != 2 {
		t.Fatal("failed notification acknowledged work")
	}
	notifier.fail = false
	if err := deliverReminders(store, notifier, pending); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 || notifier.sent[0].Body != "2 items are ready to review in Raven." {
		t.Fatal("due items not grouped")
	}
	_ = store.Close()
	store, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cards, _ = store.ListCards()
	pending, _ = reminderSnapshot(cards, now)
	if len(pending) != 0 {
		t.Fatal("restart repeated an alert")
	}
	// A new schedule must become eligible for a fresh alert.
	updated, err := store.ReviewItem(folder.ID, card.ID, card.Items[0].ID, 4)
	if err != nil {
		t.Fatal(err)
	}
	pending, _ = reminderSnapshot([]storage.Card{updated}, updated.Items[0].Schedule.Due)
	if len(pending) != 1 || pending[0].ItemID != card.Items[0].ID {
		t.Fatal("new due occurrence suppressed")
	}
}
