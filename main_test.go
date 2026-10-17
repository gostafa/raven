package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
	"go.uber.org/fx"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
	boltstore "raven/internal/shared/boltstore/adapters/bbolt"
	"raven/internal/shared/boltstore/ports"
	cffbatch "raven/internal/shared/cffbatch/adapters/cff"
	desktopmedia "raven/internal/shared/desktopmedia/adapters/wails"
	scheduling "raven/internal/shared/scheduling/adapters/fsrs"
)

func TestCompleteDependencyGraph(t *testing.T) {
	if err := fx.ValidateApp(applicationModule, fx.Invoke(func(*RavenService) {})); err != nil {
		t.Fatal(err)
	}
}

type failingRepository struct {
	ports.Repository
	failure error
}

func (r failingRepository) ListFolders() ([]folderapi.Folder, error) { return nil, r.failure }

func TestFacadeOwnsErrorContract(t *testing.T) {
	cause := fmt.Errorf("read library: %w", bolt.ErrDatabaseNotOpen)
	service := &RavenService{
		configuration: configurationFixture(),
		store:         failingRepository{failure: cause},
	}
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
	schedule, log, err := scheduling.New(&configurationFixture().Library).Initialize(now)
	if err != nil {
		t.Fatal(err)
	}
	value := cardapi.Card{
		ID:        "card",
		FolderID:  "folder",
		CreatedAt: now.UnixMilli(),
		DueAt:     schedule.Due.UnixMilli(),
		Items: []contentapi.Item{
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
				Reviews:         []contentapi.Review{{Log: log, Automatic: true}},
				LastNotifiedDue: schedule.Due.UnixMilli(),
			},
		},
	}
	for _, pair := range [][2]any{
		{value, facadeValue(t, value)},
		{folderapi.Folder{ID: "folder", Title: "Ideas", Tags: []string{}}, facadeValue(t, folderapi.Folder{ID: "folder", Title: "Ideas", Tags: []string{}})},
		{[]cardapi.Card(nil), facadeValue(t, []cardapi.Card(nil))},
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

func facadeValue[Value any](t *testing.T, value Value) Value {
	t.Helper()
	cause := errors.New("storage failure")
	response := publicResult(value, cause)
	result, err := response.value, response.failure
	var failure *ServiceError
	if !errors.As(err, &failure) || failure.Message != cause.Error() || errors.Is(err, cause) {
		t.Fatalf("public result leaked its implementation error: %v", err)
	}
	return result
}

func TestFolderServiceAndShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store, err := boltstore.Open(
		path,
		&boltstore.Dependencies{
			Batch:     cffbatch.New(),
			Scheduler: scheduling.New(&configurationFixture().Library),
			Logger:    nil,
			Settings:  *storeSettingsFixture(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := &RavenService{configuration: configurationFixture(), store: store}
	folder, err := service.CreateFolder(&folderapi.CreateInput{Title: " Ideas "})
	if err != nil {
		t.Fatal(err)
	}
	folders, err := service.ListFolders()
	if err != nil || !reflect.DeepEqual(folders, []folderapi.Folder{folder}) {
		t.Fatalf("service folders: %+v, error=%v", folders, err)
	}
	card, err := service.CreateCard(
		&cardapi.CreateInput{
			FolderID: folder.ID,
			Items: []contentapi.Draft{
				{Kind: "text", Text: "A question"},
				{Kind: "text", Text: "An answer"},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err = service.ReviewItem(
		&cardapi.ReviewInput{
			FolderID: folder.ID,
			CardID:   card.ID,
			ItemID:   card.Items[0].ID,
			Rating:   3,
		},
	)
	if err != nil || card.Items[0].Schedule.Reps != 2 || card.DueAt <= card.LastReviewed {
		t.Fatalf("service review: %+v, error=%v", card, err)
	}
	due := card.DueAt
	card, err = service.UpdateCard(
		&cardapi.UpdateInput{FolderID: folder.ID, CardID: card.ID, Items: []contentapi.Draft{
			{ID: card.Items[0].ID, Kind: "text", Text: "An edited question"},
			{ID: card.Items[1].ID, Kind: "text", Text: "An edited answer"},
		}},
	)
	if err != nil || card.DueAt != due || card.Items[0].Text != "An edited question" {
		t.Fatalf("service edit: %+v, error=%v", card, err)
	}
	cards, err := service.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card}) {
		t.Fatalf("service cards: %+v, error=%v", cards, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListFolders(); err == nil || errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("shutdown did not close storage: %v", err)
	}
	reopened, err := boltstore.Open(
		path,
		&boltstore.Dependencies{
			Batch:     cffbatch.New(),
			Scheduler: scheduling.New(&configurationFixture().Library),
			Logger:    nil,
			Settings:  *storeSettingsFixture(),
		},
	)
	if err != nil {
		t.Fatalf("shutdown did not release the database lock: %v", err)
	}
	defer reopened.Close()
	rawFolders, err := reopened.ListFolders()
	folders = rawFolders
	if err != nil || !reflect.DeepEqual(folders, []folderapi.Folder{folder}) {
		t.Fatalf("folders after service shutdown: %+v, error=%v", folders, err)
	}
	rawCards, err := reopened.ListCards()
	cards = rawCards
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card}) {
		t.Fatalf("cards after service shutdown: %+v, error=%v", cards, err)
	}
}

func TestMediaMiddlewareAndPickerErrors(t *testing.T) {
	service := &RavenService{configuration: configurationFixture()}
	next := http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) },
	)
	for _, test := range []struct {
		path   string
		status int
	}{{"/", http.StatusAccepted}, {"/media/folder/card/item", http.StatusServiceUnavailable}} {
		w := httptest.NewRecorder()
		desktopmedia.NewRegistry(&desktopmedia.Dependencies{Saved: nil, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Batch: cffbatch.New()}, mediaSettingsFixture()).
			Middleware(next).
			ServeHTTP(w, httptest.NewRequest("GET", test.path, nil))
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
	service := &RavenService{configuration: configurationFixture()}
	if _, err := service.ListFolders(); err == nil {
		t.Fatal("listing without storage should fail")
	}
	if _, err := service.CreateFolder(&folderapi.CreateInput{Title: "Ideas"}); err == nil {
		t.Fatal("creating without storage should fail")
	}
	if _, err := service.ListCards(); err == nil {
		t.Fatal("listing cards without storage should fail")
	}
	if _, err := service.CreateCard(&cardapi.CreateInput{}); err == nil {
		t.Fatal("creating cards without storage should fail")
	}
	if _, err := service.UpdateCard(&cardapi.UpdateInput{}); err == nil {
		t.Fatal("editing without storage should fail")
	}
	if _, err := service.ReviewItem(
		&cardapi.ReviewInput{FolderID: "folder", CardID: "card", ItemID: "item", Rating: 3},
	); err == nil {
		t.Fatal("reviewing cards without storage should fail")
	}
}

func TestNullFacadeInputsUseExistingValidation(t *testing.T) {
	store, err := boltstore.Open(
		filepath.Join(t.TempDir(), "raven.db"),
		&boltstore.Dependencies{
			Batch:     cffbatch.New(),
			Scheduler: scheduling.New(&configurationFixture().Library),
			Logger:    nil,
			Settings:  *storeSettingsFixture(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	service := &RavenService{configuration: configurationFixture(), store: store}
	if _, err := service.CreateFolder(nil); err == nil {
		t.Fatal("accepted empty folder input")
	}
	if _, err := service.CreateCard(nil); err == nil {
		t.Fatal("accepted empty card input")
	}
	if _, err := service.UpdateCard(nil); err == nil {
		t.Fatal("accepted empty card update")
	}
	folders, err := service.ListFolders()
	if err != nil || len(folders) != 0 {
		t.Fatalf("invalid inputs wrote records: %+v %v", folders, err)
	}
}

func configurationFixture() *configuration.Settings {
	settings, err := (configyaml.Loader{}).Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func storeSettingsFixture() *boltstore.Settings {
	settings := configurationFixture()
	return &boltstore.Settings{
		Rules:            settings.Library,
		MediaRoutePrefix: settings.MediaRoutePrefix,
		LogErrorKey:      settings.LogErrorKey,
	}
}

func mediaSettingsFixture() *desktopmedia.Settings {
	settings := configurationFixture()
	return &desktopmedia.Settings{
		MediaRoutePrefix:   settings.MediaRoutePrefix,
		PreviewRoutePrefix: settings.PreviewRoutePrefix,
		LogErrorKey:        settings.LogErrorKey,
	}
}

func TestConfigurationFacadeReturnsOwnedFrontendSnapshot(t *testing.T) {
	settings := configurationFixture()
	settings.PreferencesStorageKey = "custom.preferences"
	settings.MediaRoutePrefix = "/saved/"
	service := &RavenService{configuration: settings}
	value := service.GetConfiguration()
	if value.PreferencesStorageKey != settings.PreferencesStorageKey ||
		value.MediaRoutePrefix != settings.MediaRoutePrefix ||
		value.TextKind != settings.Library.TextKind ||
		value.SchedulesChanged != settings.SchedulesChanged ||
		value.ShowReviews != settings.ShowReviews ||
		value.OpenFloatingReview != settings.OpenFloatingReview ||
		value.CloseFloatingReview != settings.CloseFloatingReview {
		t.Fatal("configuration facade omitted or changed values", value)
	}
	value.PreferencesStorageKey = "edited"
	if service.GetConfiguration().PreferencesStorageKey != "custom.preferences" {
		t.Fatal("facade returned shared mutable state")
	}
}
