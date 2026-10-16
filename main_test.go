package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	bolt "go.etcd.io/bbolt"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	"raven/internal/library/adapters/boltstore"
	"raven/internal/library/adapters/scheduling"
	"raven/internal/library/ports"
)

type failingRepository struct {
	ports.Repository
	failure error
}

func (r failingRepository) ListFolders() ([]folderapi.Folder, error) { return nil, r.failure }

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

func TestStartupInstallsStorageAndMediaTogether(t *testing.T) {
	store, err := boltstore.Open(filepath.Join(t.TempDir(), "startup.db"), scheduling.New())
	if err != nil {
		t.Fatal(err)
	}
	service := new(RavenService)
	lifecycle := lifecycleAdapter{
		service: service,
		openStore: func() (storageSession, error) {
			return storageSession{repository: store, media: store.MediaHandler()}, nil
		},
	}
	t.Cleanup(func() {
		if closeErr := lifecycle.ServiceShutdown(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	if err = lifecycle.ServiceStartup(
		context.Background(),
		application.ServiceOptions{},
	); err != nil {
		t.Fatal(err)
	}
	if service.store != store || service.mediaHandler == nil {
		t.Fatal("startup did not install storage and its media handler")
	}
	if _, err = service.CreateFolder(&folderapi.CreateInput{Title: "Startup"}); err != nil {
		t.Fatal(err)
	}
}

func TestStartupFailureKeepsFacadeErrorAndExistingState(t *testing.T) {
	cause := fmt.Errorf("database unavailable: %w", bolt.ErrDatabaseNotOpen)
	repository := failingRepository{failure: cause}
	handler := http.NewServeMux()
	service := &RavenService{store: repository, mediaHandler: handler}
	lifecycle := lifecycleAdapter{
		service:   service,
		openStore: func() (storageSession, error) { return storageSession{}, cause },
	}
	err := lifecycle.ServiceStartup(context.Background(), application.ServiceOptions{})
	var public *ServiceError
	if !errors.As(err, &public) || public.Message != cause.Error() || errors.Unwrap(err) != nil {
		t.Fatalf("startup exposed an implementation error: %v", err)
	}
	if service.store != repository || service.mediaHandler != handler {
		t.Fatal("failed startup changed installed resources")
	}
}

func TestFacadePreservesCompleteJSONContract(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 123000000, time.UTC)
	schedule, log, err := scheduling.New().Initialize(now)
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
	result, err := publicResult(value, cause)
	var failure *ServiceError
	if !errors.As(err, &failure) || failure.Message != cause.Error() || errors.Is(err, cause) {
		t.Fatalf("public result leaked its implementation error: %v", err)
	}
	return result
}

func TestFolderServiceAndShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store, err := boltstore.Open(path, scheduling.New())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := &RavenService{store: store}
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
	if err := (&lifecycleAdapter{service: service}).ServiceShutdown(); err != nil {
		t.Fatalf("shutdown without storage: %v", err)
	}
}

func TestDesktopWindowConfigurationPreservesMonochromeBehavior(t *testing.T) {
	main := mainWindowOptions()
	if main.Name != "raven" || main.Title != "Raven" || main.URL != "/" || !main.Hidden ||
		main.Width != 960 ||
		main.Height != 680 ||
		main.MinWidth != 620 ||
		main.MinHeight != 520 {
		t.Fatalf("main window changed: %+v", main)
	}
	if main.Mac.Appearance != application.NSAppearanceNameDarkAqua ||
		main.Mac.Backdrop != application.MacBackdropTranslucent {
		t.Fatal("main window appearance changed")
	}
	widget := widgetWindowOptions()
	if widget.Name != "raven-widget" || widget.URL != "/?view=widget" || widget.Hidden ||
		widget.Width != widgetSize ||
		widget.MinWidth != widgetSize ||
		widget.MaxWidth != widgetSize ||
		widget.Height != widgetSize ||
		widget.MinHeight != widgetSize ||
		widget.MaxHeight != widgetSize {
		t.Fatal("widget geometry changed")
	}
	if widget.InitialPosition != application.WindowXY || widget.X != 24 || widget.Y != 100 ||
		widget.BackgroundType != application.BackgroundTypeTransparent ||
		!reflect.DeepEqual(widget.BackgroundColour, application.NewRGBA(255, 255, 255, 0)) {
		t.Fatal("widget background or placement changed")
	}
	if widget.Mac.Backdrop != application.MacBackdropTransparent || !widget.Mac.DisableShadow ||
		widget.Mac.CornerType != application.MacWindowCornerTypeSquare {
		t.Fatal("widget platform appearance changed")
	}
	review := reviewWindowOptions()
	if review.Name != "raven-review" || review.URL != "/?view=review" || !review.Hidden ||
		review.Width != reviewWidth ||
		review.Height != reviewHeight ||
		review.Mac.Appearance != application.NSAppearanceNameDarkAqua {
		t.Fatal("review window changed")
	}
	for _, window := range []application.WebviewWindowOptions{widget, review} {
		if !window.Frameless || !window.DisableResize || !window.AlwaysOnTop ||
			!window.Windows.HiddenOnTaskbar ||
			window.MaximiseButtonState != application.ButtonDisabled ||
			window.FullscreenButtonState != application.ButtonDisabled {
			t.Fatal("floating window interaction changed")
		}
		if window.Mac.WindowClass != application.MacWindowClassPanel ||
			!window.Mac.PanelPreferences.NonActivating ||
			!window.Mac.PanelPreferences.FloatingPanel ||
			window.Mac.WindowLevel != application.MacWindowLevelPopUpMenu {
			t.Fatal("floating panel behavior changed")
		}
	}
	for _, window := range []application.WebviewWindowOptions{main, review} {
		if !reflect.DeepEqual(window.BackgroundColour, application.NewRGB(0, 0, 0)) {
			t.Fatal("native black background changed")
		}
	}
	identity := desktopIdentityOptions()
	if identity.Name != "Raven" ||
		identity.Description != "A little practice. A little more remembered." ||
		len(identity.Icon) == 0 {
		t.Fatal("application identity changed")
	}
}

func TestNullFacadeInputsUseExistingValidation(t *testing.T) {
	store, err := boltstore.Open(filepath.Join(t.TempDir(), "raven.db"), scheduling.New())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	service := &RavenService{store: store}
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
