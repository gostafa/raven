package application_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	reviewapi "raven/api/reviews"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
	reminders "raven/internal/features/reminders/application"
	boltstore "raven/internal/shared/boltstore/adapters/bbolt"
	cffbatch "raven/internal/shared/cffbatch/adapters/cff"
	scheduling "raven/internal/shared/scheduling/adapters/fsrs"
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
	store, err := openRepository(
		path,
		boltstore.NewDependencies(
			cffbatch.New(),
			scheduling.New(new(configuration.Library(configurationFixture()))),
			storeSettingsFixture(),
		),
	)
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
	now := contentapi.Schedule(contentapi.Items(card.Items)[1].Schedule).Due.Add(time.Second)
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
	store, err = openRepository(
		path,
		boltstore.NewDependencies(
			cffbatch.New(),
			scheduling.New(new(configuration.Library(configurationFixture()))),
			storeSettingsFixture(),
		),
	)
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
			ItemID:   contentapi.Items(card.Items)[0].ID,
			Rating:   4,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	pending, _ = reminders.Snapshot(
		[]cardapi.Card{updated},
		contentapi.Schedule(contentapi.Items(updated.Items)[0].Schedule).Due,
	)
	if len(pending) != 1 || pending[0].ItemID != contentapi.Items(card.Items)[0].ID {
		t.Fatal("new due occurrence suppressed")
	}
}

func (n *fakeReminderNotifier) RequestAuthorization() (bool, error) { return n.allowed, nil }

type reminderSource struct{}

func (reminderSource) ListCards() (string, error)             { return "[]", nil }
func (reminderSource) ClaimNotificationPrompt() (bool, error) { return true, nil }
func (reminderSource) MarkNotified(string) error              { return nil }

type blockingNotifier struct {
	fakeReminderNotifier
	started chan struct{}
	release chan struct{}
}

func (n *blockingNotifier) RequestAuthorization() (bool, error) {
	close(n.started)
	<-n.release
	return n.allowed, nil
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

func configurationFixture() configuration.Settings {
	settings, err := configyaml.Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func storeSettingsFixture() *boltstore.Settings {
	settings := configurationFixture()
	return &boltstore.Settings{
		Rules:            configuration.Library(settings),
		MediaRoutePrefix: settings.MediaRoutePrefix(),
		LogErrorKey:      settings.LogErrorKey(),
	}
}

// testRepository translates public JSON records while retaining typed test assertions.
type testRepository struct{ *boltstore.Store }

func openRepository(path string, dependencies *boltstore.Dependencies) (*testRepository, error) {
	store, err := boltstore.Open(path, dependencies)
	return &testRepository{Store: store}, err
}

func encodeRecord(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func (store *testRepository) ListCards() ([]cardapi.Card, error) {
	encoded, err := store.Store.ListCards()
	if err != nil {
		return nil, err
	}
	return reminders.DecodeCards(encoded)
}

func (store *testRepository) CreateFolder(input *folderapi.CreateInput) (folderapi.Folder, error) {
	encoded, err := store.Store.CreateFolder(encodeRecord(input))
	if err != nil {
		return folderapi.Folder{}, err
	}
	return contentapi.Decode[folderapi.Folder](json.RawMessage(encoded))
}

func (store *testRepository) CreateCard(input *cardapi.CreateInput) (cardapi.Card, error) {
	return decodeCard(store.Store.CreateCard(encodeRecord(input)))
}

func (store *testRepository) ReviewItem(input *cardapi.ReviewInput) (cardapi.Card, error) {
	return decodeCard(store.Store.ReviewItem(encodeRecord(input)))
}

func decodeCard(encoded string, err error) (cardapi.Card, error) {
	if err != nil {
		return cardapi.Card{}, err
	}
	card, err := contentapi.Decode[cardapi.Card](json.RawMessage(encoded))
	if err != nil {
		return cardapi.Card{}, err
	}
	err = cardapi.Normalize(&card)
	return card, err
}
