package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"raven/internal/library"
	"raven/internal/library/adapters/boltstore"
	"raven/internal/library/adapters/scheduling"
)

type failingRepository struct {
	library.Repository
	failure error
}

func (r failingRepository) ListFolders() ([]library.Folder, error) { return nil, r.failure }

func TestFacadeOwnsErrorContract(t *testing.T) {
	cause := fmt.Errorf("read library: %w", bolt.ErrDatabaseNotOpen)
	service := &RavenService{store: failingRepository{failure: cause}}
	_, err := service.ListFolders()
	var public *ServiceError
	if !errors.As(err, &public) || public.Message != cause.Error() {
		t.Fatalf("unexpected public error: %v", err)
	}
	if errors.Is(err, cause) || errors.Is(err, bolt.ErrDatabaseNotOpen) ||
		errors.Unwrap(err) != nil {
		t.Fatal("facade retained an implementation error chain")
	}
}

func TestFacadePreservesCompleteJSONContract(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 123000000, time.UTC)
	schedule, log, err := scheduling.New().Initialize(now)
	if err != nil {
		t.Fatal(err)
	}
	value := library.Card{
		ID:        "card",
		FolderID:  "folder",
		CreatedAt: now.UnixMilli(),
		DueAt:     schedule.Due.UnixMilli(),
		Items: []library.ContentItem{
			{
				ID:              "item",
				FolderID:        "folder",
				CardID:          "card",
				Position:        0,
				Kind:            "image",
				OriginalName:    "diagram.png",
				MIMEType:        "image/png",
				ByteSize:        12,
				RelativePath:    "media/item.png",
				Schedule:        schedule,
				Reviews:         []library.ItemReview{{Log: log, Automatic: true}},
				LastNotifiedDue: schedule.Due.UnixMilli(),
			},
		},
	}
	for _, pair := range [][2]any{
		{value, publicCard(value)},
		{library.Folder{ID: "folder", Title: "Ideas", Tags: []string{}}, publicFolder(library.Folder{ID: "folder", Title: "Ideas", Tags: []string{}})},
		{[]library.Card(nil), publicCards(nil)},
	} {
		before, err := json.Marshal(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		after, err := json.Marshal(pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatalf("wire shape changed:\n%s\n%s", before, after)
		}
	}
}

func TestFolderServiceAndShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store, err := boltstore.Open(path, scheduling.New())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := &RavenService{store: store}
	folder, err := service.CreateFolder(CreateFolderInput{Title: " Ideas "})
	if err != nil {
		t.Fatal(err)
	}
	folders, err := service.ListFolders()
	if err != nil || !reflect.DeepEqual(folders, []Folder{folder}) {
		t.Fatalf("service folders: %+v, error=%v", folders, err)
	}
	card, err := service.CreateCard(
		CreateCardInput{
			FolderID: folder.ID,
			Items: []ItemDraft{
				{Kind: "text", Text: "A question"},
				{Kind: "text", Text: "An answer"},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err = service.ReviewItem(folder.ID, card.ID, card.Items[0].ID, 3)
	if err != nil || card.Items[0].Schedule.Reps != 2 || card.DueAt <= card.LastReviewed {
		t.Fatalf("service review: %+v, error=%v", card, err)
	}
	due := card.DueAt
	card, err = service.UpdateCard(
		UpdateCardInput{FolderID: folder.ID, CardID: card.ID, Items: []ItemDraft{
			{ID: card.Items[0].ID, Kind: "text", Text: "An edited question"},
			{ID: card.Items[1].ID, Kind: "text", Text: "An edited answer"},
		}},
	)
	if err != nil || card.DueAt != due || card.Items[0].Text != "An edited question" {
		t.Fatalf("service edit: %+v, error=%v", card, err)
	}
	cards, err := service.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []Card{card}) {
		t.Fatalf("service cards: %+v, error=%v", cards, err)
	}
	if err := (&lifecycleAdapter{service: service}).ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListFolders(); err == nil || errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("shutdown did not close storage: %v", err)
	}
	reopened, err := boltstore.Open(path, scheduling.New())
	if err != nil {
		t.Fatalf("shutdown did not release the database lock: %v", err)
	}
	defer reopened.Close()
	rawFolders, err := reopened.ListFolders()
	folders = publicFolders(rawFolders)
	if err != nil || !reflect.DeepEqual(folders, []Folder{folder}) {
		t.Fatalf("folders after service shutdown: %+v, error=%v", folders, err)
	}
	rawCards, err := reopened.ListCards()
	cards = publicCards(rawCards)
	if err != nil || !reflect.DeepEqual(cards, []Card{card}) {
		t.Fatalf("cards after service shutdown: %+v, error=%v", cards, err)
	}
}

func TestMediaMiddlewareAndPickerErrors(t *testing.T) {
	service := &RavenService{}
	next := http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) },
	)
	for _, test := range []struct {
		path   string
		status int
	}{{"/", http.StatusAccepted}, {"/media/folder/card/item", http.StatusServiceUnavailable}} {
		w := httptest.NewRecorder()
		service.mediaMiddleware(next).ServeHTTP(w, httptest.NewRequest("GET", test.path, nil))
		if w.Code != test.status {
			t.Fatalf("%s: got %d", test.path, w.Code)
		}
	}
	for _, kind := range []string{"image", "audio", "video", "invalid"} {
		if _, err := service.SelectMediaFiles(kind); err == nil {
			t.Fatalf("picker with unavailable window accepted %s", kind)
		}
	}
}

func TestFolderServiceWithoutStorage(t *testing.T) {
	service := &RavenService{}
	if _, err := service.ListFolders(); err == nil {
		t.Fatal("listing without storage should fail")
	}
	if _, err := service.CreateFolder(CreateFolderInput{Title: "Ideas"}); err == nil {
		t.Fatal("creating without storage should fail")
	}
	if _, err := service.ListCards(); err == nil {
		t.Fatal("listing cards without storage should fail")
	}
	if _, err := service.CreateCard(CreateCardInput{}); err == nil {
		t.Fatal("creating cards without storage should fail")
	}
	if _, err := service.UpdateCard(UpdateCardInput{}); err == nil {
		t.Fatal("editing without storage should fail")
	}
	if _, err := service.ReviewItem("folder", "card", "item", 3); err == nil {
		t.Fatal("reviewing cards without storage should fail")
	}
	if err := (&lifecycleAdapter{service: service}).ServiceShutdown(); err != nil {
		t.Fatalf("shutdown without storage: %v", err)
	}
}
