package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"raven/internal/storage"

	bolt "go.etcd.io/bbolt"
)

func TestSelectedMediaPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	service := &RavenService{}
	service.previews.Store("selected-token", path)
	handler := service.mediaMiddleware(http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodGet, "/media-preview/selected-token", nil)
	request.Header.Set("Range", "bytes=2-5")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusPartialContent || response.Body.String() != "2345" || response.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("preview range: %d %s %v", response.Code, response.Body.String(), response.Header())
	}
	for _, url := range []string{"/media-preview/unknown", "/media-preview/" + path} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, url, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("unselected path served: %s", url)
		}
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/media-preview/selected-token", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status: %d", response.Code)
	}
}

func TestFolderServiceAndShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := &RavenService{store: store}
	folder, err := service.CreateFolder(storage.CreateFolderInput{Title: " Ideas "})
	if err != nil {
		t.Fatal(err)
	}
	folders, err := service.ListFolders()
	if err != nil || !reflect.DeepEqual(folders, []storage.Folder{folder}) {
		t.Fatalf("service folders: %+v, error=%v", folders, err)
	}
	card, err := service.CreateCard(storage.CreateCardInput{FolderID: folder.ID, Items: []storage.ItemDraft{{Kind: "text", Text: "A question"}, {Kind: "text", Text: "An answer"}}})
	if err != nil {
		t.Fatal(err)
	}
	card, err = service.ReviewItem(folder.ID, card.ID, card.Items[0].ID, 3)
	if err != nil || card.Items[0].Schedule.Reps != 2 || card.DueAt <= card.LastReviewed {
		t.Fatalf("service review: %+v, error=%v", card, err)
	}
	due := card.DueAt
	card, err = service.UpdateCard(storage.UpdateCardInput{FolderID: folder.ID, CardID: card.ID, Items: []storage.ItemDraft{
		{ID: card.Items[0].ID, Kind: "text", Text: "An edited question"},
		{ID: card.Items[1].ID, Kind: "text", Text: "An edited answer"},
	}})
	if err != nil || card.DueAt != due || card.Items[0].Text != "An edited question" {
		t.Fatalf("service edit: %+v, error=%v", card, err)
	}
	cards, err := service.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []storage.Card{card}) {
		t.Fatalf("service cards: %+v, error=%v", cards, err)
	}
	if err := service.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListFolders(); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("shutdown did not close storage: %v", err)
	}
	reopened, err := storage.Open(path)
	if err != nil {
		t.Fatalf("shutdown did not release the database lock: %v", err)
	}
	defer reopened.Close()
	folders, err = reopened.ListFolders()
	if err != nil || !reflect.DeepEqual(folders, []storage.Folder{folder}) {
		t.Fatalf("folders after service shutdown: %+v, error=%v", folders, err)
	}
	cards, err = reopened.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []storage.Card{card}) {
		t.Fatalf("cards after service shutdown: %+v, error=%v", cards, err)
	}
}

func TestMediaMiddlewareAndPickerErrors(t *testing.T) {
	service := &RavenService{}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) })
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
	if _, err := service.CreateFolder(storage.CreateFolderInput{Title: "Ideas"}); err == nil {
		t.Fatal("creating without storage should fail")
	}
	if _, err := service.ListCards(); err == nil {
		t.Fatal("listing cards without storage should fail")
	}
	if _, err := service.CreateCard(storage.CreateCardInput{}); err == nil {
		t.Fatal("creating cards without storage should fail")
	}
	if _, err := service.UpdateCard(storage.UpdateCardInput{}); err == nil {
		t.Fatal("editing without storage should fail")
	}
	if _, err := service.ReviewItem("folder", "card", "item", 3); err == nil {
		t.Fatal("reviewing cards without storage should fail")
	}
	if err := service.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown without storage: %v", err)
	}
}
