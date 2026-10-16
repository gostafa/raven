package application_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	reviewapi "raven/api/reviews"
	"raven/internal/features/library/adapters/boltstore"
	"raven/internal/features/library/adapters/scheduling"
	reminders "raven/internal/features/reminders/application"
)

func TestReminderSnapshotGroupsDueItemsAndFindsNextDeadline(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	item := func(id string, due time.Time, acknowledged bool) contentapi.Item {
		value := contentapi.Item{ID: id, Schedule: reviewapi.Schedule{Due: due}}
		if acknowledged {
			value.LastNotifiedDue = due.UnixMilli()
		}
		return value
	}
	cards := []cardapi.Card{{ID: "card", FolderID: "folder", Items: []contentapi.Item{
		item(
			"overdue",
			now.Add(-time.Hour),
			false,
		),
		item("due", now, false),
		item("notified", now, true),
		item("future", now.Add(2*time.Second), false),
	}}}
	pending, delay := reminders.Snapshot(cards, now)
	if len(pending) != 2 || delay != 2*time.Second || pending[0].ItemID != "overdue" ||
		pending[1].ItemID != "due" {
		t.Fatalf("pending=%+v delay=%v", pending, delay)
	}
	// Simulate resume: future work is now overdue and gets reconciled immediately.
	pending, delay = reminders.Snapshot(cards, now.Add(time.Hour))
	if len(pending) != 3 || delay != 30*time.Second {
		t.Fatal("resume missed due items")
	}
}

type fakeReminderNotifier struct {
	allowed bool
	fail    bool
	sent    []string
}

func (n *fakeReminderNotifier) CheckNotificationAuthorization() (bool, error) { return n.allowed, nil }

func (n *fakeReminderNotifier) Send(count int) error {
	if n.fail {
		return errors.New("delivery failed")
	}
	n.sent = append(n.sent, fmt.Sprintf("%d items are ready to review in Raven.", count))
	return nil
}

func TestReminderDeliveryPermissionFailureAndRestartDeduplication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store, err := boltstore.Open(path, scheduling.New())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	folder, _ := store.CreateFolder(&folderapi.CreateInput{Title: "Due"})
	card, err := store.CreateCard(&cardapi.CreateInput{
		FolderID: folder.ID,
		Items:    []contentapi.Draft{{Kind: "text", Text: "one"}, {Kind: "text", Text: "two"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := card.Items[0].Schedule.Due.Add(time.Second)
	cards, _ := store.ListCards()
	pending, _ := reminders.Snapshot(cards, now)
	notifier := &fakeReminderNotifier{}
	if err := reminders.Deliver(store, notifier, pending); err != nil {
		t.Fatal(err)
	}
	cards, _ = store.ListCards()
	pending, _ = reminders.Snapshot(cards, now)
	if len(notifier.sent) != 0 || len(pending) != 2 {
		t.Fatal("denial sent an alert or acknowledged work")
	}
	notifier.allowed = true
	notifier.fail = true
	if err := reminders.Deliver(store, notifier, pending); err == nil {
		t.Fatal("delivery failure hidden")
	}
	cards, _ = store.ListCards()
	pending, _ = reminders.Snapshot(cards, now)
	if len(pending) != 2 {
		t.Fatal("failed notification acknowledged work")
	}
	notifier.fail = false
	if err := reminders.Deliver(store, notifier, pending); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 || notifier.sent[0] != "2 items are ready to review in Raven." {
		t.Fatal("due items not grouped")
	}
	_ = store.Close()
	store, err = boltstore.Open(path, scheduling.New())
	if err != nil {
		t.Fatal(err)
	}
	cards, _ = store.ListCards()
	pending, _ = reminders.Snapshot(cards, now)
	if len(pending) != 0 {
		t.Fatal("restart repeated an alert")
	}
	// A new schedule must become eligible for a fresh alert.
	updated, err := store.ReviewItem(
		&cardapi.ReviewInput{
			FolderID: folder.ID,
			CardID:   card.ID,
			ItemID:   card.Items[0].ID,
			Rating:   4,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	pending, _ = reminders.Snapshot([]cardapi.Card{updated}, updated.Items[0].Schedule.Due)
	if len(pending) != 1 || pending[0].ItemID != card.Items[0].ID {
		t.Fatal("new due occurrence suppressed")
	}
}

func (n *fakeReminderNotifier) RequestAuthorization() error { return nil }

type reminderSource struct{}

func (reminderSource) ListCards() ([]cardapi.Card, error)           { return nil, nil }
func (reminderSource) ClaimNotificationPrompt() (bool, error)       { return true, nil }
func (reminderSource) MarkNotified([]reviewapi.DueOccurrence) error { return nil }

type blockingNotifier struct {
	fakeReminderNotifier
	started chan struct{}
	release chan struct{}
}

func (n *blockingNotifier) RequestAuthorization() error {
	close(n.started)
	<-n.release
	return nil
}

func TestCoordinatorShutdownDoesNotWaitForAuthorization(t *testing.T) {
	notifier := &blockingNotifier{started: make(chan struct{}), release: make(chan struct{})}
	c := reminders.Start(
		reminderSource{},
		notifier,
		reminders.Callbacks{Publish: func() {}, Warn: func(string, error) {}},
	)
	defer func() { close(notifier.release); c.Stop() }()
	select {
	case <-notifier.started:
	case <-time.After(5 * time.Second):
		t.Fatal("authorization did not start")
	}
	stopped := make(chan struct{})
	go func() { c.Wake(); c.Stop(); c.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("authorization blocked shutdown")
	}
	// Wake remains safe after cancellation, including a late authorization result.
	c.Wake()
}
