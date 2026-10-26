package bbolt

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/multierr"

	cardapi "raven/api/cards"
	transferapi "raven/api/cardtransfer"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	reviewapi "raven/api/reviews"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
	archivezip "raven/internal/features/folderarchive/adapters/zipfile"
	archive "raven/internal/features/folderarchive/domain"
	archiveports "raven/internal/features/folderarchive/ports"
	library "raven/internal/shared/boltstore/domain"
	libraryports "raven/internal/shared/boltstore/ports"
	cacheotter "raven/internal/shared/cache/adapters/otter"
	cffbatch "raven/internal/shared/cffbatch/adapters/cff"
	resilience "raven/internal/shared/resilience/adapters/retry"
	scheduling "raven/internal/shared/scheduling/adapters/fsrs"
)

func enableReadCaches(t testing.TB, storage *persistence) {
	t.Helper()
	storage.snapshots = startedCache(
		t,
		&cacheotter.Options[string, string]{
			MaximumWeight: 16 * 1024 * 1024,
			Weigher:       func(_ string, value string) uint32 { return uint32(len(value)) },
		},
	)
	storage.mediaLookup = startedCache(
		t,
		&cacheotter.Options[string, libraryports.MediaRecord]{MaximumSize: 4096},
	)
	storage.chunks = startedCache(
		t,
		&cacheotter.Options[string, []byte]{
			MaximumWeight: 128 * 1024 * 1024,
			Weigher:       func(_ string, value []byte) uint32 { return uint32(len(value)) },
		},
	)
}

func TestReadCachesFollowEveryLibraryMutation(t *testing.T) {
	storage := openTestStore(t)
	enableReadCaches(t, storage)
	repository := &Store{state: storage}
	fresh := func() {
		t.Helper()
		wantCards, err := encodeResponse(storeListCards(storage))
		if err != nil {
			t.Fatal(err)
		}
		gotCards, err := repository.ListCards()
		if err != nil || gotCards != wantCards {
			t.Fatal("stale cards", err)
		}
		wantFolders, err := encodeResponse(storeListFolders(storage))
		if err != nil {
			t.Fatal(err)
		}
		gotFolders, err := repository.ListFolders()
		if err != nil || gotFolders != wantFolders {
			t.Fatal("stale folders", err)
		}
	}
	fresh()
	folder, err := storeCreateFolder(storage, &folderapi.CreateInput{Title: "First"})
	if err != nil {
		t.Fatal(err)
	}
	fresh()
	other, err := storeCreateFolder(storage, &folderapi.CreateInput{Title: "Destination"})
	if err != nil {
		t.Fatal(err)
	}
	fresh()
	if _, err := storeUpdateFolder(
		storage,
		&folderapi.UpdateInput{ID: folder.ID, Title: "Updated"},
	); err != nil {
		t.Fatal(err)
	}
	fresh()
	card, err := storeCreateCard(
		storage,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("Before")},
	)
	if err != nil {
		t.Fatal(err)
	}
	fresh()
	item := contentapi.Items(card.Items)[0]
	card, err = storeUpdateCard(
		storage,
		&cardapi.UpdateInput{
			FolderID: folder.ID,
			CardID:   card.ID,
			Items:    []contentapi.Draft{{ID: item.ID, Kind: "text", Text: "After"}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	fresh()
	if err := storeMarkNotified(
		storage,
		[]reviewapi.DueOccurrence{
			{
				FolderID: folder.ID,
				CardID:   card.ID,
				ItemID:   item.ID,
				Due:      contentapi.Schedule(item.Schedule).Due.UnixMilli(),
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	fresh()
	for _, operation := range []string{"copy", "move"} {
		if _, err := storeTransferCards(
			storage,
			&transferapi.TransferInput{
				Operation:           operation,
				DestinationFolderID: other.ID,
				Sources: []transferapi.Reference{
					{FolderID: folder.ID, CardID: card.ID},
				},
			},
		); err != nil {
			t.Fatal(err)
		}
		fresh()
	}
	manifest := `{"version":1,"exported_at":"2026-01-01T00:00:00Z","folders":[{"folder":{"id":"old","title":"Imported","description":"","tags":[]},"cards":[]}]}`
	if _, err := repository.ImportFolderCopies(manifest, "fresh", nil); err != nil {
		t.Fatal(err)
	}
	fresh()
	if err := storeDeleteFolder(storage, other.ID); err != nil {
		t.Fatal(err)
	}
	fresh()
	if err := storeDeleteFolder(storage, folder.ID); err != nil {
		t.Fatal(err)
	}
	fresh()
}

func TestMediaCacheRejectsMovedAndRemovedItems(t *testing.T) {
	for _, operation := range []string{"remove", "move", "delete-folder"} {
		t.Run(operation, func(t *testing.T) {
			storage := openTestStore(t)
			enableReadCaches(t, storage)
			folder, _ := storeCreateFolder(storage, &folderapi.CreateInput{Title: "Source"})
			other, _ := storeCreateFolder(storage, &folderapi.CreateInput{Title: "Destination"})
			image, _ := mediaFixture(t, ".png")
			card, err := storeCreateCard(
				storage,
				&cardapi.CreateInput{
					FolderID: folder.ID,
					Items: []contentapi.Draft{
						{Kind: "image", SourcePath: image},
						{Kind: "text", Text: "Keep"},
					},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			item := contentapi.Items(card.Items)[0]
			url := "/media/" + folder.ID + "/" + card.ID + "/" + item.ID
			handler := storeMediaHandler(storage)
			warm := httptest.NewRecorder()
			handler.ServeHTTP(warm, httptest.NewRequest(http.MethodGet, url, nil))
			if warm.Code != http.StatusOK {
				t.Fatal(warm.Code)
			}
			switch operation {
			case "remove":
				_, err = storeUpdateCard(
					storage,
					&cardapi.UpdateInput{
						FolderID: folder.ID,
						CardID:   card.ID,
						Items:    textItems("Keep"),
					},
				)
			case "move":
				_, err = storeTransferCards(
					storage,
					&transferapi.TransferInput{
						Operation:           "move",
						DestinationFolderID: other.ID,
						Sources: []transferapi.Reference{
							{FolderID: folder.ID, CardID: card.ID},
						},
					},
				)
			case "delete-folder":
				err = storeDeleteFolder(storage, folder.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, url, nil)
			request.Header.Set("If-None-Match", warm.Header().Get("ETag"))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusNotFound {
				t.Fatal("stale ownership authorized", response.Code)
			}
		})
	}
}

func startedCache[Key comparable, Value any](
	t testing.TB,
	options *cacheotter.Options[Key, Value],
) *cacheotter.Cache[Key, Value] {
	t.Helper()
	cache := cacheotter.New(options)
	if err := cache.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cache.Close)
	return cache
}

func TestReadCachesReuseAndInvalidateCommittedData(t *testing.T) {
	storage := openTestStore(t)
	enableReadCaches(t, storage)
	repository := &Store{state: storage}
	folder, err := storeCreateFolder(storage, &folderapi.CreateInput{Title: "Cached"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := storeCreateCard(
		storage,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("Remember")},
	)
	if err != nil {
		t.Fatal(err)
	}
	originalView := storage.view
	var views atomic.Int64
	storage.view = func(callback func(*bolt.Tx) error) error { views.Add(1); return originalView(callback) }
	first, err := repository.ListCards()
	if err != nil {
		t.Fatal(err)
	}
	folders, err := repository.ListFolders()
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if got, err := repository.ListCards(); err != nil || got != first {
			t.Fatal(got, err)
		}
		if got, err := repository.ListFolders(); err != nil || got != folders {
			t.Fatal(got, err)
		}
	}
	if views.Load() != 2 {
		t.Fatalf("warm listings used %d views", views.Load())
	}
	failure := errors.New("rollback")
	if err := storage.update(func(*bolt.Tx) error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := repository.ListCards(); err != nil || views.Load() != 2 {
		t.Fatal("rollback evicted valid snapshot", err)
	}
	item := contentapi.Items(card.Items)[0]
	if _, err := storeReviewItem(
		storage,
		&cardapi.ReviewInput{FolderID: folder.ID, CardID: card.ID, ItemID: item.ID, Rating: 3},
	); err != nil {
		t.Fatal(err)
	}
	if got, err := repository.ListCards(); err != nil || got == first {
		t.Fatal("review left stale snapshot", err)
	}
	if err := storeClose(storage); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ListCards(); err == nil {
		t.Fatal("closed library returned a cached response")
	}
}

func TestConcurrentListingCannotRestoreDataAfterCommit(t *testing.T) {
	storage := openTestStore(t)
	enableReadCaches(t, storage)
	repository := &Store{state: storage}
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	load := func(*persistence) (string, error) { close(started); <-release; return "old", nil }
	go func() {
		defer close(done)
		if _, err := cachedListing(storage, "cards", load); err != nil {
			t.Error(err)
		}
	}()
	<-started
	committed := make(chan struct{})
	go func() {
		defer close(committed)
		if _, err := storeCreateFolder(storage, &folderapi.CreateInput{Title: "New"}); err != nil {
			t.Error(err)
		}
	}()
	close(release)
	<-done
	<-committed
	if got, err := repository.ListCards(); err != nil || got != "[]" {
		t.Fatalf("stale cache restored: %q %v", got, err)
	}
}

func TestCachedMediaValidatorsRangesAndOwnership(t *testing.T) {
	storage := openTestStore(t)
	enableReadCaches(t, storage)
	folder, _ := storeCreateFolder(storage, &folderapi.CreateInput{Title: "Media"})
	image, data := mediaFixture(t, ".png")
	card, err := storeCreateCard(
		storage,
		&cardapi.CreateInput{
			FolderID: folder.ID,
			Items:    []contentapi.Draft{{Kind: "image", SourcePath: image}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	item := contentapi.Items(card.Items)[0]
	url := "/media/" + folder.ID + "/" + card.ID + "/" + item.ID
	handler := storeMediaHandler(storage)
	request := func(method, byteRange, etag string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, url, nil)
		r.Header.Set("Range", byteRange)
		r.Header.Set("If-None-Match", etag)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	w := request(http.MethodGet, "", "")
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), data) ||
		w.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatal("initial media", w.Code)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing validator")
	}
	originalView := storage.view
	storage.view = func(func(*bolt.Tx) error) error { return errors.New("warm lookup touched database") }
	if got := request(
		http.MethodGet,
		"",
		etag,
	); got.Code != http.StatusNotModified ||
		got.Body.Len() != 0 {
		t.Fatal("conditional", got.Code)
	}
	if got := request(http.MethodHead, "", ""); got.Code != http.StatusOK || got.Body.Len() != 0 {
		t.Fatal("head", got.Code)
	}
	if got := request(
		http.MethodGet,
		"bytes=-3",
		"",
	); got.Code != http.StatusPartialContent ||
		!bytes.Equal(got.Body.Bytes(), data[len(data)-3:]) {
		t.Fatal("suffix range", got.Code)
	}
	if got := request(
		http.MethodGet,
		"bytes=0-1,4-5",
		"",
	); got.Code != http.StatusPartialContent ||
		!strings.HasPrefix(got.Header().Get("Content-Type"), "multipart/byteranges") {
		t.Fatal("multipart", got.Code)
	}
	if got := request(
		http.MethodGet,
		"bytes=999999-",
		"",
	); got.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatal("invalid range", got.Code)
	}
	storage.view = originalView
	if err := storeDeleteCard(storage, folder.ID, card.ID); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodGet, "", etag); got.Code != http.StatusNotFound {
		t.Fatal("deleted media served from cache", got.Code)
	}
	// Lookup failures are retried rather than retained.
	if got := request(http.MethodGet, "", ""); got.Code != http.StatusNotFound {
		t.Fatal(got.Code)
	}
}

func TestNewReviewPartsHaveStaggeredDueTimes(t *testing.T) {
	scheduler := scheduling.New(new(configuration.Library(configurationFixture())))
	storage := &persistence{initializeSchedule: scheduler.Initialize}
	write := &cardWrite{card: cardapi.Card{}, items: []contentapi.Item{
		{ID: "first"}, {ID: "hint", Hint: true}, {ID: "second"}, {ID: "third"},
	}}
	if err := cardWriteMergeSchedules(storage, write); err != nil {
		t.Fatal(err)
	}
	items := contentapi.Items(cardRecord(&write.card).Items)
	first := contentapi.Schedule(items[0].Schedule)
	for index, position := range []int{0, 2, 3} {
		schedule := contentapi.Schedule(items[position].Schedule)
		if got, want := schedule.Due.Sub(
			first.Due,
		), time.Duration(
			index,
		)*5*time.Minute; got != want {
			t.Fatalf("part %d due offset = %v, want %v", position, got, want)
		}
		if !schedule.LastReview.Equal(first.LastReview) {
			t.Fatal("spacing changed the automatic review time")
		}
	}
	previous := contentapi.Schedule(items[2].Schedule)
	write.items = items
	if err := cardWriteMergeSchedules(storage, write); err != nil {
		t.Fatal(err)
	}
	if got := contentapi.Schedule(
		contentapi.Items(cardRecord(&write.card).Items)[2].Schedule,
	); got != previous {
		t.Fatal("saving existing parts changed their schedule")
	}
}

func TestJSONRepositoryRoundTripAndLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	dependencies := NewDependencies(
		cffbatch.New(),
		scheduling.New(new(configuration.Library(configurationFixture()))),
		storeSettingsFixture(),
	)
	repository := New(path, dependencies)
	if err := repository.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	folderJSON, err := repository.CreateFolder(`{"title":"JSON library","tags":["plain"]}`)
	if err != nil {
		t.Fatal(err)
	}
	var folder folderapi.Folder
	if err := json.Unmarshal([]byte(folderJSON), &folder); err != nil {
		t.Fatal(err)
	}
	if folders, err := repository.ListFolders(); err != nil ||
		!strings.Contains(folders, `"JSON library"`) {
		t.Fatalf("folder response: %s %v", folders, err)
	}
	request := encodeFixture(
		t,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("Prompt", "Answer")},
	)
	cardJSON, err := repository.CreateCard(string(request))
	if err != nil {
		t.Fatal(err)
	}
	card := decodeCardResponse(t, cardJSON)
	if cards, err := repository.ListCards(); err != nil || !strings.Contains(cards, `"schedule"`) {
		t.Fatalf("card response: %s %v", cards, err)
	}
	updatedFolder, err := repository.UpdateFolder(string(encodeFixture(t, &folderapi.UpdateInput{
		Title:       " Updated library ",
		Description: " Context ",
		Tags:        []string{"edited"},
		ID:          folder.ID,
	})))
	if err != nil || !strings.Contains(updatedFolder, `"title":"Updated library"`) {
		t.Fatalf("update folder: %s %v", updatedFolder, err)
	}
	if unchanged, failure := repository.ListCards(); failure != nil ||
		unchanged != "["+cardJSON+"]" {
		t.Fatalf("folder edit changed cards: %s %v", unchanged, failure)
	}
	if _, failure := repository.UpdateFolder(`{"id":"missing","title":"Valid"}`); failure == nil {
		t.Fatal("updated a missing folder")
	}
	if _, failure := repository.UpdateFolder(
		string(encodeFixture(t, &folderapi.UpdateInput{ID: folder.ID})),
	); failure == nil {
		t.Fatal("accepted an empty folder title")
	}
	items := contentapi.Items(card.Items)
	updatedJSON, err := repository.UpdateCard(string(encodeFixture(t, &cardapi.UpdateInput{
		FolderID: folder.ID,
		CardID:   card.ID,
		Items: []contentapi.Draft{
			{ID: items[0].ID, Kind: "text", Text: "Updated"},
			{ID: items[1].ID, Kind: "text", Text: "Answer"},
		},
	})))
	if err != nil {
		t.Fatal(err)
	}
	updated := decodeCardResponse(t, updatedJSON)
	if !reflect.DeepEqual(
		contentapi.Schedule(items[0].Schedule),
		contentapi.Schedule(contentapi.Items(updated.Items)[0].Schedule),
	) {
		t.Fatal("JSON update replaced the retained schedule")
	}
	occurrence := notificationOccurrence(&updated, 0)
	if err := repository.MarkNotified(
		string(encodeFixture(t, []reviewapi.DueOccurrence{occurrence})),
	); err != nil {
		t.Fatal(err)
	}
	reviewJSON, err := repository.ReviewItem(string(encodeFixture(t, &cardapi.ReviewInput{
		FolderID: folder.ID, CardID: card.ID, ItemID: items[0].ID, Rating: 3,
	})))
	if err != nil {
		t.Fatal(err)
	}
	reviewed := decodeCardResponse(t, reviewJSON)
	if len(contentapi.Reviews(contentapi.Items(reviewed.Items)[0].Reviews)) != 2 {
		t.Fatal("review history did not cross JSON boundary")
	}
	for _, claim := range []func() (bool, error){repository.ClaimNotificationPrompt, repository.ClaimAutostartInitialization} {
		if claimed, err := claim(); err != nil || !claimed {
			t.Fatalf("first claim: %v %v", claimed, err)
		}
	}
	response := httptest.NewRecorder()
	repository.MediaHandler().
		ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/media/missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("media route status: %d", response.Code)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.DeleteCard(folder.ID, card.ID); err != nil {
		t.Fatal(err)
	}
	if err := reopened.DeleteFolder(folder.ID); err != nil {
		t.Fatal(err)
	}
	if folders, err := reopened.ListFolders(); err != nil || folders != "[]" {
		t.Fatalf("empty array compatibility: %s %v", folders, err)
	}
}

func archiveFixture(t *testing.T) (*Store, *archive.Document, string) {
	t.Helper()
	state := openTestStore(t)
	store := &Store{state: state}
	folder, err := storeCreateFolder(
		state,
		&folderapi.CreateInput{
			Title:       "Memories 世界",
			Description: "Portable",
			Tags:        []string{"study"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := storeCreateFolder(state, &folderapi.CreateInput{Title: "Empty"})
	if err != nil {
		t.Fatal(err)
	}
	drafts := []contentapi.Draft{
		{Kind: "text", Text: "مرحبا 🌍"},
		{Kind: "text", Text: "Hint", Hint: true},
	}
	for _, media := range []struct{ kind, ext string }{{"image", ".png"}, {"image", ".png"}, {"audio", ".wav"}, {"video", ".mp4"}} {
		filename, _ := mediaFixture(t, media.ext)
		drafts = append(drafts, contentapi.Draft{Kind: media.kind, SourcePath: filename})
	}
	card, err := storeCreateCard(state, &cardapi.CreateInput{FolderID: folder.ID, Items: drafts})
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-7 * 24 * time.Hour)
	for index := range contentapi.Items(card.Items) {
		if err = library.InitializeItem(
			&contentapi.Items(card.Items)[index],
			past,
			state.initializeSchedule,
		); err != nil {
			t.Fatal(err)
		}
		contentapi.Items(card.Items)[index].LastNotifiedDue = 123
	}
	if err = library.ApplyReview(
		&contentapi.Items(card.Items)[0],
		&reviewapi.Attempt{At: past.Add(time.Hour), Rating: 3},
		&library.ReviewDependencies{
			Rules: settingsValue(state.settings).Rules,
			Next:  state.nextSchedule,
		},
	); err != nil {
		t.Fatal(err)
	}
	err = state.update(func(tx *bolt.Tx) error {
		bucket, failure := cardBucket(tx, folder.ID)
		if failure != nil {
			return failure
		}
		return writeCardRecords(tx, bucket, &card)
	})
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "folders.raven.zip")
	var document *archive.Document
	err = store.SnapshotFolders(
		[]string{folder.ID, empty.ID},
		func(manifest string, open archiveports.OpenMedia) error {
			value, decodeErr := archive.Decode(manifest)
			if decodeErr != nil {
				return decodeErr
			}
			document = value
			items, err := contentapi.Decode[[]contentapi.Item](document.Folders[0].Cards[0].Items)
			if err != nil {
				return err
			}
			for index := range items {
				if err = contentapi.NormalizeItem(&items[index]); err != nil {
					return err
				}
			}
			document.Folders[0].Cards[0].Items = items
			return archivezip.New().Write(filename, manifest, open)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return store, document, filename
}

func TestFolderArchiveRoundTripFreshAndResume(t *testing.T) {
	store, document, filename := archiveFixture(t)
	before, err := store.ListCards()
	if err != nil {
		t.Fatal(err)
	}
	original := document.Folders[0].Cards[0]
	originalItems := contentapi.Items(original.Items)
	for _, mode := range []string{archive.Resume, archive.Fresh, archive.Resume} {
		started := time.Now().UnixMilli()
		var ids []string
		err = archivezip.New().
			Read(filename, func(manifest string, open archiveports.OpenMedia) error {
				var failure error
				ids, failure = store.ImportFolderCopies(manifest, mode, open)
				return failure
			})
		if err != nil || len(ids) != 2 {
			t.Fatalf("%s import: %v %v", mode, ids, err)
		}
		cards, err := storeListCards(store.state)
		if err != nil {
			t.Fatal(err)
		}
		var imported cardapi.Card
		for _, card := range cards {
			if card.FolderID == ids[0] {
				imported = card
			}
		}
		if imported.ID == original.ID || imported.CreatedAt != original.CreatedAt {
			t.Fatal("card identity or creation time", imported)
		}
		if mode == archive.Resume &&
			(imported.DueAt != original.DueAt || imported.LastReviewed != original.LastReviewed || imported.DueAt >= started) {
			t.Fatal("review aggregates changed", imported)
		}
		if mode == archive.Fresh && (imported.LastReviewed != 0 || imported.DueAt < started) {
			t.Fatal("fresh review aggregates", imported)
		}
		for index, item := range contentapi.Items(imported.Items) {
			previous := originalItems[index]
			if item.ID == previous.ID || item.CardID != imported.ID || item.FolderID != ids[0] ||
				item.LastNotifiedDue != 0 ||
				item.Hint != previous.Hint ||
				item.Text != previous.Text ||
				item.OriginalName != previous.OriginalName {
				t.Fatal("content ownership or metadata", item)
			}
			if mode == archive.Resume {
				if !reflect.DeepEqual(
					contentapi.Schedule(item.Schedule),
					contentapi.Schedule(previous.Schedule),
				) ||
					!reflect.DeepEqual(
						contentapi.Reviews(item.Reviews),
						contentapi.Reviews(previous.Reviews),
					) {
					t.Fatal("resume state changed", item)
				}
			} else if history := contentapi.Reviews(item.Reviews); len(history) != 1 || !history[0].Automatic || contentapi.Schedule(item.Schedule).LastReview.UnixMilli() < started {
				t.Fatal("fresh state retained manual reviews", item)
			}
			if item.RelativePath != "" {
				oldBytes, failure := os.ReadFile(storeMediaPath(store.state, previous.RelativePath))
				if failure != nil {
					t.Fatal(failure)
				}
				newBytes, failure := os.ReadFile(storeMediaPath(store.state, item.RelativePath))
				if failure != nil || !bytes.Equal(newBytes, oldBytes) ||
					item.RelativePath == previous.RelativePath {
					t.Fatal("media copy", failure)
				}
			}
		}
	}
	current, err := storeListCards(store.state)
	if err != nil {
		t.Fatal(err)
	}
	for _, card := range current {
		if card.FolderID == original.FolderID && "["+string(encodeFixture(t, &card))+"]" != before {
			t.Fatal("original card changed")
		}
	}
	if folders, err := storeListFolders(store.state); err != nil || len(folders) != 8 {
		t.Fatalf("copy imports: %d %v", len(folders), err)
	}
}

func TestFolderArchiveValidationAndRollback(t *testing.T) {
	store, document, filename := archiveFixture(t)
	before, _ := store.ListFolders()
	entries, err := os.ReadDir(store.state.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("archive media failed")
	for _, mode := range []string{"unknown", archive.Resume, archive.Fresh} {
		if _, err = store.ImportFolderCopies(
			string(encodeFixture(t, document)),
			mode,
			func(string) (io.ReadCloser, error) { return nil, failure },
		); err == nil {
			t.Fatal("accepted failed import")
		}
	}
	originalUpdate := store.state.update
	store.state.update = func(func(*bolt.Tx) error) error { return failure }
	err = archivezip.New().
		Read(filename, func(manifest string, open archiveports.OpenMedia) error {
			_, err := store.ImportFolderCopies(manifest, archive.Resume, open)
			return err
		})
	store.state.update = originalUpdate
	if !errors.Is(err, failure) {
		t.Fatal("lost database failure", err)
	}
	after, _ := store.ListFolders()
	remaining, err := os.ReadDir(store.state.mediaDir)
	if err != nil || before != after || len(remaining) != len(entries) {
		t.Fatal("partial import survived", err)
	}
	for _, mutate := range []func(*archive.Document){
		func(value *archive.Document) { value.Version = 9 },
		func(value *archive.Document) { value.Folders[0].Metadata.Title = "" },
		func(value *archive.Document) { value.Folders = append(value.Folders, value.Folders[0]) },
		func(value *archive.Document) { value.Folders[0].Cards[0].FolderID = "wrong" },
		func(value *archive.Document) { value.Folders[0].Cards[0].Items = "bad" },
		func(value *archive.Document) { value.Folders[0].Cards[0].Items = []contentapi.Item{} },
	} {
		copyDocument, err := archive.Decode(string(encodeFixture(t, document)))
		if err != nil {
			t.Fatal(err)
		}
		mutate(copyDocument)
		if _, err = store.ImportFolderCopies(
			string(encodeFixture(t, copyDocument)),
			archive.Resume,
			nil,
		); err == nil {
			t.Fatal("accepted invalid records")
		}
	}
}

func TestFolderArchiveSnapshotSurvivesConcurrentReview(t *testing.T) {
	store, document, _ := archiveFixture(t)
	original := document.Folders[0].Cards[0]
	err := store.SnapshotFolders(
		[]string{original.FolderID},
		func(manifest string, _ archiveports.OpenMedia) error {
			value, decodeErr := archive.Decode(manifest)
			if decodeErr != nil {
				return decodeErr
			}
			_, err := store.ReviewItem(
				string(
					encodeFixture(
						t,
						&cardapi.ReviewInput{
							FolderID: original.FolderID,
							CardID:   original.ID,
							ItemID:   contentapi.Items(original.Items)[0].ID,
							Rating:   3,
						},
					),
				),
			)
			if err != nil {
				return err
			}
			if value.Folders[0].Cards[0].LastReviewed != original.LastReviewed {
				t.Fatal("snapshot changed during review")
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{{}, {""}, {"missing"}, {original.FolderID, original.FolderID}} {
		if err = store.SnapshotFolders(ids, func(string, archiveports.OpenMedia) error {
			t.Fatal("invalid snapshot consumed")
			return nil
		}); err == nil {
			t.Fatal("invalid selection", ids)
		}
	}
}

func TestFolderArchiveInvalidItemsAndHistory(t *testing.T) {
	store, document, _ := archiveFixture(t)
	manifest := string(encodeFixture(t, document))
	if _, err := store.ImportFolderCopies("invalid", archive.Resume, nil); err == nil {
		t.Fatal("invalid JSON")
	}
	for _, mutate := range []func(*contentapi.Item){
		func(item *contentapi.Item) { item.Position = 99 },
		func(item *contentapi.Item) { item.CardID = "wrong" },
		func(item *contentapi.Item) { item.Text = "" },
		func(item *contentapi.Item) {
			item.ByteSize = archive.MaximumMediaSize + 1
			item.Kind = "image"
			item.Text = ""
			item.OriginalName = "large.png"
			item.MIMEType = "image/png"
			item.RelativePath = "media/" + item.ID + ".png"
		},
		func(item *contentapi.Item) { item.Schedule = "invalid" },
		func(item *contentapi.Item) { item.Reviews = "invalid" },
		func(item *contentapi.Item) { item.Reviews = []contentapi.Review{} },
		func(item *contentapi.Item) { item.Reviews = []contentapi.Review{{Log: "invalid"}} },
		func(item *contentapi.Item) { item.Reviews = []contentapi.Review{{Log: reviewapi.Log{}}} },
	} {
		transfer := folderImport{storage: store.state, mode: archive.Resume}
		value, err := archive.Decode(manifest)
		if err != nil {
			t.Fatal(err)
		}
		items, err := contentapi.Decode[[]contentapi.Item](value.Folders[0].Cards[0].Items)
		if err != nil {
			t.Fatal(err)
		}
		mutate(&items[0])
		value.Folders[0].Cards[0].Items = items
		if err = folderImportPrepare(&transfer, string(encodeFixture(t, value))); err == nil {
			t.Fatal("invalid archive item accepted")
		}
	}
	previous := store.state.validateSchedule
	store.state.validateSchedule = func(any) error { return io.ErrUnexpectedEOF }
	if _, err := store.ImportFolderCopies(
		manifest,
		archive.Resume,
		nil,
	); !errors.Is(
		err,
		io.ErrUnexpectedEOF,
	) {
		t.Fatal(err)
	}
	store.state.validateSchedule = previous
	previousInitialize := store.state.initializeSchedule
	store.state.initializeSchedule = func(time.Time) (json.RawMessage, json.RawMessage, error) { return nil, nil, io.ErrUnexpectedEOF }
	if _, err := store.ImportFolderCopies(
		manifest,
		archive.Fresh,
		nil,
	); !errors.Is(
		err,
		io.ErrUnexpectedEOF,
	) {
		t.Fatal(err)
	}
	store.state.initializeSchedule = previousInitialize
}

func TestFolderArchiveMediaAndSnapshotFailures(t *testing.T) {
	store, document, _ := archiveFixture(t)
	for _, name := range []string{"../bad", "media/missing.png"} {
		if _, err := openArchiveMedia(store.state, name); err == nil {
			t.Fatal("invalid media open")
		}
	}
	bad := &archive.Document{
		Folders: []archive.Folder{{Cards: []cardapi.Card{{Items: make(chan int)}}}},
	}
	if err := consumeFolderSnapshot(store.state, bad, nil); err == nil {
		t.Fatal("invalid snapshot JSON")
	}
	transfer := folderImport{storage: store.state}
	media := contentapi.Items(document.Folders[0].Cards[0].Items)[2]
	if err := folderImportPersistMedia(
		&transfer,
		&media,
		filepath.Join(t.TempDir(), "missing"),
	); err == nil {
		t.Fatal("missing staging file")
	}
	badFile := filepath.Join(t.TempDir(), "bad.png")
	if err := os.WriteFile(badFile, []byte("not a PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := folderImportPersistMedia(&transfer, &media, badFile); err == nil {
		t.Fatal("invalid media signature")
	}
	file, err := os.Open(storeMediaPath(store.state, media.RelativePath))
	if err != nil {
		t.Fatal(err)
	}
	media.MIMEType = "image/jpeg"
	if err = folderImportInspectMedia(&transfer, &media, file); err == nil {
		t.Fatal("MIME mismatch")
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if err = folderImportCommitMedia(&transfer, &media, "missing"); err == nil {
		t.Fatal("missing staged media commit")
	}
	file, err = os.CreateTemp(t.TempDir(), "copy")
	if err != nil {
		t.Fatal(err)
	}
	if err = copyArchiveMedia(file, strings.NewReader("short"), 99); err == nil {
		t.Fatal("media size mismatch")
	}
	previousDir := store.state.mediaDir
	store.state.mediaDir = filepath.Join(t.TempDir(), "missing")
	if err = folderImportCopyMedia(&transfer, &media, strings.NewReader("x")); err == nil {
		t.Fatal("missing media directory")
	}
	store.state.mediaDir = previousDir
	media.ByteSize = 99
	if err = folderImportCopyMedia(&transfer, &media, strings.NewReader("short")); err == nil {
		t.Fatal("copy size mismatch")
	}
	if err = store.state.update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(foldersBucket)).
			Bucket([]byte(document.Folders[0].Metadata.ID)).
			DeleteBucket([]byte(itemsBucket))
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.SnapshotFolders(
		[]string{document.Folders[0].Metadata.ID},
		func(string, archiveports.OpenMedia) error { return nil },
	); err == nil {
		t.Fatal("corrupt folder snapshot")
	}
}

func TestFolderArchiveAllocationAndWriteFailures(t *testing.T) {
	store, document, _ := archiveFixture(t)
	entropy := rand.Reader
	rand.Reader = failedEntropy{}
	t.Cleanup(func() { rand.Reader = entropy })
	transfer := folderImport{storage: store.state, document: document, mode: archive.Resume}
	if err := folderImportStageCard(
		&transfer,
		&document.Folders[0].Cards[0],
	); !errors.Is(
		err,
		io.ErrUnexpectedEOF,
	) {
		t.Fatal(err)
	}
	err := store.state.update(func(tx *bolt.Tx) error { return folderImportWrite(&transfer, tx) })
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	err = store.state.update(
		func(tx *bolt.Tx) error { return folderImportWriteCards(tx, &document.Folders[0]) },
	)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	rand.Reader = entropy
	invalid := cardapi.Card{FolderID: document.Folders[0].Metadata.ID}
	err = store.state.update(func(tx *bolt.Tx) error { return writeArchiveCard(tx, &invalid) })
	if err == nil {
		t.Fatal("empty imported card persisted")
	}
	err = store.state.update(
		func(tx *bolt.Tx) error { return writeArchiveCardRecords(tx, &cardapi.Card{FolderID: "missing"}) },
	)
	if err == nil {
		t.Fatal("missing folder write")
	}
	err = store.state.view(
		func(tx *bolt.Tx) error { return allocateArchiveFolder(tx, &folderapi.Folder{}) },
	)
	if err == nil {
		t.Fatal("read-only folder allocation")
	}
	badDocument := archive.Document{
		Folders: []archive.Folder{
			{Metadata: document.Folders[0].Metadata, Cards: []cardapi.Card{invalid}},
		},
	}
	transfer.document = &badDocument
	err = store.state.update(func(tx *bolt.Tx) error { return folderImportWrite(&transfer, tx) })
	if err == nil {
		t.Fatal("partial folder commit")
	}
}

func encodeFixture(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func decodeCardResponse(t *testing.T, input string) cardapi.Card {
	t.Helper()
	var card cardapi.Card
	if err := contentapi.DecodeJSON([]byte(input), &card); err != nil {
		t.Fatal(err)
	}
	if err := cardapi.Normalize(&card); err != nil {
		t.Fatal(err)
	}
	return card
}

func TestRepositoryPreservesLargeIntegerContentAcrossReopen(t *testing.T) {
	storage := openTestStore(t)
	folder, err := storeCreateFolder(storage, &folderapi.CreateInput{Title: "Exact integers"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := storeCreateCard(
		storage,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("Keep counters")},
	)
	if err != nil {
		t.Fatal(err)
	}
	item := &contentapi.Items(card.Items)[0]
	schedule := contentapi.Schedule(item.Schedule)
	schedule.Reps, schedule.Lapses = math.MaxUint64, 9007199254740993
	item.Schedule, item.LastNotifiedDue = schedule, int64(math.MaxInt64)
	history := contentapi.Reviews(item.Reviews)
	log := contentapi.Log(history[0].Log)
	log.ScheduledDays = math.MaxUint64
	history[0].Log = log
	if err := storage.update(
		func(transaction *bolt.Tx) error { return putItems(transaction, &card) },
	); err != nil {
		t.Fatal(err)
	}
	if err := storeClose(storage); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(storage.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storeClose(reopened) })
	repository := &Store{state: reopened}
	encoded, err := repository.ListCards()
	if err != nil {
		t.Fatal(err)
	}
	var records []json.RawMessage
	if err := json.Unmarshal([]byte(encoded), &records); err != nil {
		t.Fatal(err)
	}
	decoded := decodeCardResponse(t, string(records[0]))
	if !reflect.DeepEqual(contentapi.Items(decoded.Items), contentapi.Items(card.Items)) {
		t.Fatalf("persisted integer content changed: %#v != %#v", decoded.Items, card.Items)
	}
}

func TestRepositoryJSONDecodersPreserveIntegerSchemaAndDocumentBoundaries(t *testing.T) {
	data := []byte(
		`{"schedule":{"reps":18446744073709551615},"reviews":[],"last_notified_due":9223372036854775807,"side":"prompt"}`,
	)
	item, err := decodeJSON[contentapi.Item](data)
	if err != nil {
		t.Fatal(err)
	}
	if err := contentapi.NormalizeItem(&item); err != nil {
		t.Fatal(err)
	}
	legacy, err := decodeLegacyRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, decoded := range []contentapi.Item{item, legacyContent(legacy.Item)} {
		if decoded.LastNotifiedDue != math.MaxInt64 ||
			contentapi.Schedule(decoded.Schedule).Reps != math.MaxUint64 {
			t.Fatalf("stored JSON changed exact integer state: %#v", decoded)
		}
	}
	for _, invalid := range []string{"", `{} {}`, `{} trailing`} {
		if _, err := decodeJSON[contentapi.Item]([]byte(invalid)); err == nil {
			t.Fatalf("accepted a malformed request document: %q", invalid)
		}
		if err := decodeRecord([]byte(invalid), new(contentapi.Item)); err == nil {
			t.Fatalf("accepted a malformed stored document: %q", invalid)
		}
	}
}

func TestJSONRepositoryRejectsMalformedRecordsWithoutWrites(t *testing.T) {
	state := openTestStore(t)
	repository := &Store{state: state}
	for _, change := range []func(string) (string, error){repository.CreateFolder, repository.CreateCard, repository.UpdateCard, repository.ReviewItem} {
		if _, err := change("{"); err == nil {
			t.Fatal("accepted malformed request")
		}
	}
	if err := repository.MarkNotified(`[{"Due":"invalid"}]`); err == nil {
		t.Fatal("accepted malformed acknowledgment")
	}
	if _, err := repository.CreateFolder(`{"title":""}`); err == nil {
		t.Fatal("accepted invalid folder")
	}
	if folders, err := repository.ListFolders(); err != nil || folders != "[]" {
		t.Fatalf("failed request wrote data: %s %v", folders, err)
	}
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dependencies := NewDependencies(
		cffbatch.New(),
		scheduling.New(new(configuration.Library(configurationFixture()))),
		storeSettingsFixture(),
	)
	if _, err := Open(filepath.Join(path, "raven.db"), dependencies); err == nil {
		t.Fatal("opened a database beneath a regular file")
	}
}

func TestRepositoryCodecRejectsInvalidRuntimeValues(t *testing.T) {
	if _, err := encodeResponse(math.NaN(), nil); err == nil {
		t.Fatal("accepted non-JSON response")
	}
	if _, err := encodeResponse("", io.ErrUnexpectedEOF); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if _, _, err := importMediaRecord(nil, json.RawMessage("{"), ""); err == nil {
		t.Fatal("accepted malformed media record")
	}
	if _, err := encodeRecord(&contentapi.Item{Schedule: make(chan int)}); err == nil {
		t.Fatal("accepted invalid stored schedule")
	}
	if settingsValue(nil).MediaRoutePrefix != "" || rulesValue(make(chan int)).TextKind != "" {
		t.Fatal("invalid config acquired defaults")
	}
	if cardRecords(nil) != nil || folderRecords(nil) != nil || indexedRecords(nil) != nil ||
		itemRecord(nil) != nil {
		t.Fatal("uninitialized records gained values")
	}
}

type failedEntropy struct{}

func (failedEntropy) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type failedDatabase struct{ database }

func (failedDatabase) Close() error { return io.ErrUnexpectedEOF }

type shortSnapshot struct{}

func (shortSnapshot) WriteTo(io.Writer) (int64, error) { return 1, nil }
func (shortSnapshot) Size() int64                      { return 2 }

type failedDurableFile struct{ closed bool }

func (*failedDurableFile) Sync() error       { return io.ErrUnexpectedEOF }
func (file *failedDurableFile) Close() error { file.closed = true; return os.ErrClosed }

type failedScheduler struct{ libraryports.Scheduler }

func (failedScheduler) Initialize(time.Time) (json.RawMessage, json.RawMessage, error) {
	return nil, nil, io.ErrUnexpectedEOF
}

func TestStorageBoundariesPreserveEncodingEntropyAndDurabilityFailures(t *testing.T) {
	if err := writeBackupSnapshot(
		shortSnapshot{},
		nil,
	); !errors.Is(
		err,
		errIncompleteMigrationBackup,
	) {
		t.Fatal(err)
	}
	file := &failedDurableFile{}
	if err := syncAndClose(
		file,
	); !errors.Is(err, io.ErrUnexpectedEOF) || !errors.Is(err, os.ErrClosed) ||
		!file.closed {
		t.Fatalf("sync and close: %v", err)
	}
	if err := storeClose(
		func() *persistence { var state persistence; bindDatabase(&state, failedDatabase{}); return &state }(),
	); !errors.Is(
		err,
		io.ErrUnexpectedEOF,
	) {
		t.Fatal(err)
	}
	if err := writeRecord(nil, nil, make(chan int)); err == nil {
		t.Fatal("accepted unencodable record")
	}
	synced, err := os.CreateTemp(t.TempDir(), "durable-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := syncAndClose(synced); err != nil {
		t.Fatal(err)
	}
	entropy := rand.Reader
	rand.Reader = failedEntropy{}
	t.Cleanup(func() { rand.Reader = entropy })
	if _, err := newItemID(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if _, err := legacyTextItems(
		storeSettingsFixture(),
		[]byte(`{"prompt":"Prompt","answer":"Answer"}`),
	); !errors.Is(
		err,
		io.ErrUnexpectedEOF,
	) {
		t.Fatal(err)
	}
}

func encodeLegacyItem(value any, side string) ([]byte, error) {
	fields, err := contentapi.Decode[map[string]any](value)
	if err != nil {
		return nil, err
	}
	result := make(map[string]any, len(fields)+1)
	for name := range fields {
		result[name] = fields[name]
	}
	result["side"] = side
	return json.Marshal(result)
}

func TestLegacyJSONCodecPreservesFlattenedRecords(t *testing.T) {
	content := contentapi.Item{
		ID:       "item",
		FolderID: "folder",
		CardID:   "card",
		Kind:     "text",
		Text:     "remember",
	}
	original := legacyItem{Item: content, Side: legacyPromptSide}
	data, err := encodeLegacyItem(original.Item, original.Side)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["id"] != content.ID || fields["text"] != content.Text ||
		fields["side"] != legacyPromptSide ||
		fields["Item"] != nil {
		t.Fatal("legacy content JSON was nested or renamed", string(data))
	}
	restored, err := decodeLegacyRecord(data)
	if err != nil || legacyContent(restored.Item).ID != content.ID ||
		restored.Side != original.Side {
		t.Fatal("flattened legacy JSON did not round trip", restored, err)
	}
	if data, err := encodeLegacyItem(
		nil,
		legacyAnswerSide,
	); err != nil ||
		string(data) != `{"side":"answer"}` {
		t.Fatal("empty legacy record did not preserve its side", string(data), err)
	}
	for _, value := range []any{make(chan struct{}), map[string]any{"invalid": make(chan struct{})}} {
		if _, err := encodeLegacyItem(value, legacyPromptSide); err == nil {
			t.Fatal("unsupported legacy fields produced JSON")
		}
	}
	for _, data := range []string{"{", `{"side":12}`, `{"schedule":"invalid"}`} {
		if _, err := decodeLegacyRecord([]byte(data)); err == nil {
			t.Fatal("invalid flattened legacy record was accepted", data)
		}
	}
	if legacyContent("invalid").ID != "" {
		t.Fatal("invalid legacy content produced a record")
	}
	fields = map[string]any{"id": "item"}
	if _, err := encodeLegacyItem(fields, legacyPromptSide); err != nil || fields["side"] != nil {
		t.Fatal("serializing legacy content mutated its source", fields, err)
	}
}

func TestMigrationRejectsInvalidLoadedJSONContent(t *testing.T) {
	migration := &schemaMigration{loadItems: func(*bolt.Tx, string, string) ([]any, error) {
		return []any{"invalid"}, nil
	}}
	if err := schemaMigrationLoadScheduledContent(migration, nil, &cardapi.Card{}); err == nil {
		t.Fatal("invalid JSON-valued migrated content was accepted")
	}
}

func TestLegacyCorruptionAndFailedScheduleInitialization(t *testing.T) {
	if compareLegacyItems(
		&legacyItem{Side: legacyPromptSide},
		&legacyItem{Side: legacyAnswerSide},
	) >= 0 {
		t.Fatal("legacy prompts must precede answers")
	}
	if compareLegacyItems(
		&legacyItem{Side: legacyAnswerSide},
		&legacyItem{Side: legacyPromptSide},
	) <= 0 {
		t.Fatal("legacy answers must follow prompts")
	}
	if compareLegacyItems(
		&legacyItem{Side: legacyPromptSide, Item: &contentapi.Item{Position: 0}},
		&legacyItem{Side: legacyPromptSide, Item: &contentapi.Item{Position: 1}},
	) >= 0 {
		t.Fatal("legacy items on the same side must retain their order")
	}
	store := openTestStore(t)
	migration := newSchemaMigration(
		store,
		&schemaState{version: schemaV1, existing: true},
		time.Time{},
	)
	for _, data := range []string{"{", `{"prompt":"","answer":"Answer"}`} {
		if _, err := legacyTextItems(settingsValue(store.settings), []byte(data)); err == nil {
			t.Fatal("accepted invalid legacy text")
		}
	}
	migration.version = "unsupported"
	if _, err := schemaMigrationUnscheduledItems(migration,
		nil,
		&cardapi.Card{},
	); !errors.Is(
		err,
		errUnsupportedDatabaseSchema,
	) {
		t.Fatal(err)
	}
	if _, err := mergeLegacyItems(
		settingsValue(store.settings),
		nil,
		&cardapi.Card{},
	); !errors.Is(
		err,
		errMissingLegacyCardContent,
	) {
		t.Fatal(err)
	}
	card := &cardapi.Card{ID: "card", FolderID: "folder"}
	for _, data := range []string{"{", `{"id":"wrong"}`, `{"id":"item","folder_id":"folder","card_id":"card","side":"prompt","kind":"text","text":""}`} {
		if _, err := settingsReadLegacyItem(
			settingsValue(store.settings),
			&legacyEntry{id: []byte("item"), value: []byte(data)},
			card,
		); err == nil {
			t.Fatalf("accepted corrupt item %s", data)
		}
	}
	if err := validateLegacyOrder(
		[]*legacyItem{{Item: &contentapi.Item{Position: 0}, Side: legacyPromptSide}},
	); !errors.Is(
		err,
		errMissingLegacyCardContent,
	) {
		t.Fatal(err)
	}
	store.initializeSchedule = (failedScheduler{}).Initialize
	migration.version = schemaV1
	migration.initialize = store.initializeSchedule
	card.Items = []contentapi.Item{{ID: "item", Kind: "text", Text: "Prompt"}}
	if err := schemaMigrationInitializeSchedules(
		migration,
		card,
	); !errors.Is(
		err,
		io.ErrUnexpectedEOF,
	) {
		t.Fatal(err)
	}
	write := &cardWrite{items: contentapi.Items(card.Items)}
	if err := cardWritePersistPreparedCard(
		store,
		write,
		nil,
	); !errors.Is(
		err,
		io.ErrUnexpectedEOF,
	) {
		t.Fatal(err)
	}
}

func TestCorruptMigrationBucketsAndSchemaCommit(t *testing.T) {
	store := openTestStore(t)
	if err := store.update(func(transaction *bolt.Tx) error {
		root := transaction.Bucket([]byte(foldersBucket))
		folder, err := root.CreateBucket([]byte("legacy"))
		if err != nil {
			return err
		}
		cards, err := folder.CreateBucket([]byte(cardsBucket))
		if err != nil {
			return err
		}
		migration := newSchemaMigration(
			store,
			&schemaState{version: schemaV2, existing: true},
			time.Time{},
		)
		if err := schemaMigrationMigrateFolder(migration,
			nil,
			&legacyEntry{value: []byte("invalid")},
		); !errors.Is(
			err,
			errInvalidLegacyFolder,
		) {
			t.Errorf("invalid folder: %v", err)
		}
		if _, err := schemaMigrationPrepareFolder(migration,
			folder,
			nil,
		); !errors.Is(
			err,
			errMissingLegacyContentBucket,
		) {
			t.Errorf("missing legacy content: %v", err)
		}
		migration.version = schemaV1
		if err := folder.Put([]byte(itemsBucket), []byte("scalar")); err != nil {
			return err
		}
		if _, err := schemaMigrationPrepareFolder(migration,
			folder,
			nil,
		); !errors.Is(
			err,
			bolt.ErrIncompatibleValue,
		) {
			t.Errorf("scalar legacy content: %v", err)
		}
		if err := cards.Put([]byte("broken"), []byte("{")); err != nil {
			return err
		}
		if _, err := readLegacyCard(cards, []byte("legacy"), []byte("broken")); err == nil {
			t.Error("accepted corrupt legacy metadata")
		}
		if _, err := legacyTextContent(
			settingsValue(store.settings),
			folder,
			&cardapi.Card{ID: "broken"},
		); err == nil {
			t.Error("accepted corrupt legacy text")
		}
		if err := folder.Delete([]byte(itemsBucket)); err != nil {
			return err
		}
		items, err := folder.CreateBucket([]byte(itemsBucket))
		if err != nil {
			return err
		}
		content, err := items.CreateBucket([]byte("broken"))
		if err != nil {
			return err
		}
		if err := content.Put([]byte("item"), []byte("{")); err != nil {
			return err
		}
		if _, err := mergeLegacyItems(
			settingsValue(store.settings),
			content,
			&cardapi.Card{ID: "broken"},
		); err == nil {
			t.Error("accepted corrupt item bucket")
		}
		metadata := transaction.Bucket([]byte(metaBucket))
		if err := metadata.Delete([]byte(schemaKey)); err != nil {
			return err
		}
		if _, err := metadata.CreateBucket([]byte(schemaKey)); err != nil {
			return err
		}
		if err := root.DeleteBucket([]byte("legacy")); err != nil {
			return err
		}
		if err := schemaMigrationApply(
			migration,
			transaction,
		); !errors.Is(
			err,
			bolt.ErrIncompatibleValue,
		) {
			t.Errorf("schema collision: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestFailedImportsAndCleanupPreserveFilesystemErrors(t *testing.T) {
	store := openTestStore(t)
	path, _ := mediaFixture(t, ".png")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	item := &contentapi.Item{ID: strings.Repeat("a", 32), Kind: "image"}
	store.mediaDir = filepath.Join(t.TempDir(), "missing")
	if _, _, err := storeImportOpenedMedia(
		store,
		&mediaImport{file: file, item: item, source: path},
	); !errors.Is(
		err,
		os.ErrNotExist,
	) {
		t.Fatal(err)
	}
	if item.RelativePath != "" || item.MIMEType != "" || item.ByteSize != 0 {
		t.Fatal("failed media import mutated caller metadata", item)
	}
	if _, err := openDatabase(filepath.Join(path, "child", "raven.db"), bolt.Open); err == nil {
		t.Fatal("accepted file as database parent")
	}
	temp, err := os.CreateTemp(t.TempDir(), "staging-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := temp.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stageMediaFile(temp, file, 0); !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	temp, err = os.CreateTemp(t.TempDir(), "staging-*")
	if err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	item.ByteSize = info.Size()
	item.RelativePath = "media/item.png"
	store.mediaDir = t.TempDir()
	if err := os.Mkdir(filepath.Join(store.mediaDir, "item.png"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := storeStageAndCommitMedia(
		store,
		temp,
		&mediaImport{file: file, item: item},
	); err == nil {
		t.Fatal("renamed media over directory")
	}
	target := filepath.Join(t.TempDir(), "target.png")
	store.mediaDir = filepath.Join(t.TempDir(), "missing")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := syncMediaDirectory(store, target, func(string) error {
		return io.ErrUnexpectedEOF
	}); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed commit retained media")
	}
	store.mediaDir = t.TempDir()
	stale := filepath.Join(store.mediaDir, strings.Repeat("a", 32)+".png")
	if err := os.WriteFile(stale, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(store.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(stale); err != nil {
		t.Fatal(err)
	}
	if err := storeCleanMediaEntry(store, entries[0], nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := storeCleanMediaEntries(store, entries, nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := storeClose(store); err != nil {
		t.Fatal(err)
	}
	if err := storeCleanupMedia(store); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatal(err)
	}
	if path, err := DefaultPath(); err != nil || path == "" {
		t.Fatalf("default path=%q error=%v", path, err)
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AppData", "")
	if _, err := DefaultPath(); err == nil {
		t.Fatal("missing user directory accepted")
	}
}

func TestMalformedCardContentAndWriteFailures(t *testing.T) {
	store := openTestStore(t)
	folder, err := storeCreateFolder(store, &folderapi.CreateInput{Title: "Corrupt"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := storeCreateCard(store,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("Prompt", "Answer")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.update(func(transaction *bolt.Tx) error {
		parent := transaction.Bucket([]byte(foldersBucket)).Bucket([]byte(folder.ID))
		metadata, items := parent.Bucket([]byte(cardsBucket)), parent.Bucket([]byte(itemsBucket))
		if err := initializeFolder(parent, &folder); !errors.Is(err, bolt.ErrBucketExists) {
			t.Errorf("duplicate content bucket: %v", err)
		}
		migration := newSchemaMigration(
			store,
			&schemaState{version: schemaV4, existing: true},
			time.Time{},
		)
		expected := card
		expected.Items = append(
			append([]contentapi.Item{}, contentapi.Items(card.Items)...),
			contentapi.Items(card.Items)[0],
		)
		if err := schemaMigrationVerifyCard(migration,
			transaction,
			&expected,
		); !errors.Is(
			err,
			errMissingCardContent,
		) {
			t.Errorf("missing migrated item: %v", err)
		}
		bad := card
		bad.Items = append([]contentapi.Item{}, contentapi.Items(card.Items)...)
		badSchedule := contentapi.Schedule(contentapi.Items(bad.Items)[0].Schedule)
		badSchedule.Stability = math.NaN()
		contentapi.Items(bad.Items)[0].Schedule = badSchedule
		if err := putItemsInRoot(items, &bad); err == nil {
			t.Error("accepted unencodable schedule")
		}
		contentapi.Items(bad.Items)[0].Schedule = contentapi.Schedule(
			contentapi.Items(card.Items)[0].Schedule,
		)
		contentapi.Items(bad.Items)[0].Position = 3
		if err := schemaMigrationPersistCard(migration,
			&migrationFolder{parent: parent, items: items},
			&bad,
		); err == nil {
			t.Error("accepted invalid migrated order")
		}
		if err := cardListingRead(store, (&cardListing{}), transaction); err == nil {
			t.Error("listed invalid content order")
		}
		write := &cardWrite{folderID: folder.ID, card: bad}
		if err := cardWriteWriteRecords(store, write,
			&cardTransaction{transaction: transaction, bucket: metadata}); err == nil {
			t.Error("accepted invalid saved content order")
		}
		if err := writeCardRecords(transaction, metadata, &card); err != nil {
			return err
		}
		if err := metadata.Put([]byte(card.ID), []byte(`{"id":"wrong"}`)); err != nil {
			return err
		}
		if _, err := readCardMetadata(metadata, folder.ID, card.ID); err == nil {
			t.Error("accepted wrong card ownership")
		}
		if err := writeCardRecords(transaction, metadata, &card); err != nil {
			return err
		}
		if err := metadata.Delete([]byte(card.ID)); err != nil {
			return err
		}
		if _, err := metadata.CreateBucket([]byte(card.ID)); err != nil {
			return err
		}
		if err := writeCardBuckets(
			metadata,
			items,
			&card,
		); !errors.Is(
			err,
			bolt.ErrIncompatibleValue,
		) {
			t.Errorf("migration metadata bucket: %v", err)
		}
		if err := writeCardRecords(
			transaction,
			metadata,
			&card,
		); !errors.Is(
			err,
			bolt.ErrIncompatibleValue,
		) {
			t.Errorf("card metadata bucket: %v", err)
		}
		if err := deleteOwnedCard(parent, card.ID); !errors.Is(err, bolt.ErrIncompatibleValue) {
			t.Errorf("metadata deletion: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.view(func(transaction *bolt.Tx) error {
		write := &cardWrite{folderID: "missing"}
		if err := cardWritePersist(
			store,
			write,
			transaction,
		); !errors.Is(
			err,
			errFolderNotFoundOrInvalid,
		) {
			t.Errorf("missing destination: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReadOnlyReviewAndPreparedCardStages(t *testing.T) {
	store := openTestStore(t)
	folder, err := storeCreateFolder(store, &folderapi.CreateInput{Title: "Stages"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := storeCreateCard(store,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("Prompt", "Answer")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.view(func(transaction *bolt.Tx) error {
		parent := transaction.Bucket([]byte(foldersBucket)).Bucket([]byte(folder.ID))
		metadata, items := parent.Bucket([]byte(cardsBucket)), parent.Bucket([]byte(itemsBucket))
		migration := newSchemaMigration(
			store,
			&schemaState{version: schemaV4, existing: true},
			time.Time{},
		)
		if err := schemaMigrationMigrateCard(migration,
			&migrationFolder{parent: parent, items: items, id: []byte(folder.ID)},
			[]byte(card.ID),
		); !errors.Is(
			err,
			bolt.ErrTxNotWritable,
		) {
			t.Errorf("read-only migration: %v", err)
		}
		write := &cardWrite{
			folderID: folder.ID,
			cardID:   card.ID,
			items:    contentapi.Items(card.Items),
		}
		if err := cardWritePersist(
			store,
			write,
			transaction,
		); !errors.Is(
			err,
			bolt.ErrTxNotWritable,
		) {
			t.Errorf("read-only prepared card: %v", err)
		}
		review := &cardRating{
			folderID: folder.ID,
			cardID:   card.ID,
			itemID:   contentapi.Items(card.Items)[0].ID,
			rating:   3,
			now:      contentapi.Schedule(contentapi.Items(card.Items)[0].Schedule).Due,
		}
		if err := cardRatingApply(
			store,
			review,
			transaction,
		); !errors.Is(
			err,
			bolt.ErrTxNotWritable,
		) {
			t.Errorf("read-only review: %v", err)
		}
		review.item = &contentapi.Item{}
		if err := cardRatingPersistItem(
			store,
			review,
			transaction,
		); err == nil {
			t.Error("accepted missing reviewed schedule")
		}
		if _, err := storeLookupMedia(store,
			transaction,
			[]string{folder.ID, card.ID, "missing"},
		); !errors.Is(
			err,
			errMediaNotFound,
		) {
			t.Errorf("missing media: %v", err)
		}
		_ = metadata
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := cardRatingHydrate(&cardRating{}); err == nil {
		t.Fatal("hydrated an empty reviewed card")
	}
	if err := store.update(func(transaction *bolt.Tx) error {
		root := transaction.Bucket([]byte(foldersBucket))
		if err := root.Put([]byte("scalar"), []byte("invalid")); err != nil {
			return err
		}
		if err := cardListingAppendFolder(store, (&cardListing{root: root}),
			&[two][]byte{
				[]byte("scalar"),
				[]byte("invalid"),
			},
		); !errors.Is(
			err,
			errInvalidFolderBucket,
		) {
			t.Errorf("scalar folder: %v", err)
		}
		parent := root.Bucket([]byte(folder.ID))
		metadata := parent.Bucket([]byte(cardsBucket))
		if err := metadata.Put(
			[]byte(card.ID),
			[]byte(
				fmt.Sprintf(
					`{"id":%q,"folder_id":%q,"prompt":"Prompt","answer":"Answer"}`,
					card.ID,
					folder.ID,
				),
			),
		); err != nil {
			return err
		}
		store.initializeSchedule = (failedScheduler{}).Initialize
		migration := newSchemaMigration(store, &schemaState{version: schemaV1}, time.Time{})
		if err := schemaMigrationPrepareCardContent(migration,
			parent,
			&card,
		); !errors.Is(
			err,
			io.ErrUnexpectedEOF,
		) {
			t.Errorf("migrated schedule: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var cards *bolt.Bucket
	if err := store.view(func(transaction *bolt.Tx) error {
		cards = transaction.Bucket([]byte(foldersBucket)).
			Bucket([]byte(folder.ID)).
			Bucket([]byte(cardsBucket))
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := schemaMigrationMigrateFolderCards((&schemaMigration{}),
		nil,
		cards,
	); !errors.Is(
		err,
		bolt.ErrTxClosed,
	) {
		t.Fatalf("closed migration: %v", err)
	}
}

func TestPersistentMarkersRejectReadOnlyAndCorruptBuckets(t *testing.T) {
	store := openTestStore(t)
	if err := store.view(func(transaction *bolt.Tx) error {
		for _, claim := range []func(*bolt.Tx) (bool, error){claimLoginSetting, claimNotificationSetting} {
			if _, err := claim(transaction); !errors.Is(err, bolt.ErrTxNotWritable) {
				t.Errorf("read-only marker: %v", err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.update(func(transaction *bolt.Tx) error {
		if err := transaction.DeleteBucket([]byte(metaBucket)); err != nil {
			return err
		}
		if _, err := claimNotificationSetting(transaction); !errors.Is(err, errMissingMetadata) {
			t.Errorf("missing metadata: %v", err)
		}
		settings, err := transaction.CreateBucketIfNotExists([]byte(settingsBucket))
		if err != nil {
			return err
		}
		_, err = settings.CreateBucket([]byte(autostartInitializedKey))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := storeClaimAutostartInitialization(
		store,
	); !errors.Is(
		err,
		bolt.ErrIncompatibleValue,
	) {
		t.Fatalf("corrupt login marker: %v", err)
	}
	if _, err := storeClaimNotificationPrompt(store); !errors.Is(err, errMissingMetadata) {
		t.Fatalf("missing prompt metadata: %v", err)
	}
	if err := storeClose(store); err != nil {
		t.Fatal(err)
	}
	if _, err := storeClaimNotificationPrompt(store); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatal(err)
	}
}

func TestMigrationBackupFailuresPreserveCauses(t *testing.T) {
	store := openTestStore(t)
	migration := newSchemaMigration(
		store,
		&schemaState{version: schemaV1, existing: true},
		time.Time{},
	)
	missing := filepath.Join(t.TempDir(), "missing", "raven.db")
	if err := schemaMigrationBackup(migration, missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup path: %v", err)
	}
	file, err := os.CreateTemp(t.TempDir(), "backup-*")
	if err != nil {
		t.Fatal(err)
	}
	err = schemaMigrationFinishBackup(migration, file, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = syncMigrationBackupDirectory(filepath.Dir(missing), func(string) error {
		return os.ErrNotExist
	})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup directory sync: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := syncDirectory(filepath.Dir(missing)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing directory sync: %v", err)
		}
	}
	if err := store.update(func(transaction *bolt.Tx) error {
		return transaction.Bucket([]byte(metaBucket)).Put([]byte(schemaKey), []byte(schemaV4))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := storePlanMigration(store, missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("migration backup: %v", err)
	}
	file, err = os.CreateTemp(t.TempDir(), "closed-backup-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := schemaMigrationFinishBackup(
		migration,
		file,
		t.TempDir(),
	); !errors.Is(
		err,
		os.ErrClosed,
	) {
		t.Fatalf("closed backup: %v", err)
	}
	if err := storeClose(store); err != nil {
		t.Fatal(err)
	}
	if err := schemaMigrationBackup(migration,
		filepath.Join(t.TempDir(), "raven.db"),
	); !errors.Is(
		err,
		bolt.ErrDatabaseNotOpen,
	) {
		t.Fatalf("closed database backup: %v", err)
	}
	state := schemaState{version: schemaV2}
	if err := schemaStateValidate(&state); !errors.Is(err, errMissingFoldersBucket) {
		t.Fatal(err)
	}
}

func TestReadOnlyFolderAndCardWritesPreserveTransactionFailures(t *testing.T) {
	store := openTestStore(t)
	folder, err := storeCreateFolder(store, &folderapi.CreateInput{Title: "Read-only"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := storeCreateCard(store,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("prompt", "answer")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.view(func(transaction *bolt.Tx) error {
		root := transaction.Bucket([]byte(foldersBucket))
		bucket := root.Bucket([]byte(folder.ID))
		metadata, items := bucket.Bucket([]byte(cardsBucket)), bucket.Bucket([]byte(itemsBucket))
		migration := newSchemaMigration(
			store,
			&schemaState{version: schemaV4, existing: true},
			time.Time{},
		)
		for name, operation := range map[string]func() error{
			"migration metadata":     func() error { return schemaMigrationApply(migration, transaction) },
			"migration root":         func() error { return schemaMigrationMigrateRoot(migration, transaction) },
			"folder sequence":        func() error { return assignFolderID(root, &folderapi.Folder{}) },
			"new folder":             func() error { return persistNewFolder(transaction, &folderapi.Folder{Title: "New"}) },
			"folder metadata":        func() error { return writeFolderMetadata(bucket, &folder) },
			"folder metadata update": func() error { return updateFolderMetadata(transaction, &folder) },
			"folder initialization":  func() error { return initializeFolder(bucket, &folder) },
			"card deletion":          func() error { return deleteOwnedCard(bucket, card.ID) },
			"card record deletion":   func() error { return storeDeleteCardRecords(store, transaction, &[two]string{folder.ID, card.ID}) },
			"item replacement":       func() error { _, err := replaceItemBucket(items, card.ID); return err },
			"item allocation":        func() error { _, err := replaceItemBucket(items, "new"); return err },
			"card metadata":          func() error { return putCard(metadata, &card) },
			"card content":           func() error { return putItems(transaction, &card) },
			"card buckets":           func() error { return writeCardBuckets(metadata, items, &card) },
			"card records":           func() error { return writeCardRecords(transaction, metadata, &card) },
			"card allocation":        func() error { _, err := allocateCard(metadata, folder.ID); return err },
			"migration persistence": func() error {
				return schemaMigrationPersistCard(migration, &migrationFolder{parent: bucket, items: items}, &card)
			},
			"notification acknowledgment": func() error {
				return storeMarkOccurrences(store, transaction, []reviewapi.DueOccurrence{{FolderID: folder.ID, CardID: card.ID, ItemID: contentapi.Items(card.Items)[0].ID, Due: contentapi.Schedule(contentapi.Items(card.Items)[0].Schedule).Due.UnixMilli()}})
			},
		} {
			if err := operation(); !errors.Is(err, bolt.ErrTxNotWritable) {
				t.Errorf("%s: %v", name, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestFolderUpdatePreservesWriteFailureAndMetadata(t *testing.T) {
	store := openTestStore(t)
	folder, err := storeCreateFolder(store, &folderapi.CreateInput{Title: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	update := store.update
	store.update = store.view
	t.Cleanup(func() { store.update = update })
	_, err = storeUpdateFolder(store, &folderapi.UpdateInput{
		Title: "Replacement",
		ID:    folder.ID,
	})
	if !errors.Is(err, bolt.ErrTxNotWritable) {
		t.Fatalf("lost metadata write failure: %v", err)
	}
	folders, err := storeListFolders(store)
	if err != nil || !reflect.DeepEqual(folders, []folderapi.Folder{folder}) {
		t.Fatalf("failed folder update changed metadata: %+v %v", folders, err)
	}
}

func TestCorruptLibraryRootsAndExhaustedSequences(t *testing.T) {
	store := openTestStore(t)
	if err := store.update(func(transaction *bolt.Tx) error {
		root := transaction.Bucket([]byte(foldersBucket))
		if err := root.SetSequence(math.MaxUint64); err != nil {
			return err
		}
		if _, err := nextFolderID(root); !errors.Is(err, errFolderIDSequenceExhausted) {
			t.Errorf("folder sequence: %v", err)
		}
		if _, err := allocateCard(root, "folder"); err != nil {
			t.Errorf("ordinary card allocation: %v", err)
		}
		if err := root.SetSequence(math.MaxUint64); err != nil {
			return err
		}
		if _, err := allocateCard(root, "folder"); !errors.Is(err, errCardIDSequenceExhausted) {
			t.Errorf("card sequence: %v", err)
		}
		if err := root.Put([]byte("invalid"), []byte("value")); err != nil {
			return err
		}
		if _, err := readFolder(root, []byte("invalid"), []byte("value")); err == nil {
			t.Error("accepted folder scalar")
		}
		if err := transaction.DeleteBucket([]byte(foldersBucket)); err != nil {
			return err
		}
		for _, operation := range []func() error{
			func() error { return folderListingRead((&folderListing{}), transaction) },
			func() error { return cardListingRead(store, (&cardListing{}), transaction) },
			func() error { return persistNewFolder(transaction, &folderapi.Folder{}) },
			func() error { return validateDestinationFolder(transaction, "folder") },
		} {
			if err := operation(); !errors.Is(err, errMissingFoldersBucket) {
				t.Errorf("missing folder root: %v", err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{`{"id":"wrong","title":"Title"}`, `{"id":"one","title":""}`} {
		if _, err := decodeFolder([]byte(data), "one"); !errors.Is(err, errInvalidFolderMetadata) {
			t.Fatal(err)
		}
	}
	folder, err := decodeFolder([]byte(`{"id":"one","title":"Title"}`), "one")
	if err != nil || folder.Tags == nil {
		t.Fatalf("legacy tags: %+v %v", folder, err)
	}
	if _, err := decodeCardMetadata(
		[]byte(`{"id":"wrong","folder_id":"folder"}`),
		"folder",
		"card",
	); !errors.Is(
		err,
		errInvalidCardMetadata,
	) {
		t.Fatal(err)
	}
	if err := cardContentReaderLoad(
		(&cardContentReader{}),
		nil,
	); !errors.Is(
		err,
		errMissingCardContent,
	) {
		t.Fatal(err)
	}
	if _, err := storeUpdateCard(
		store,
		&cardapi.UpdateInput{},
	); !errors.Is(
		err,
		errCardIDIsRequired,
	) {
		t.Fatal(err)
	}
}

func TestMediaSignaturesAndFailedFileOperations(t *testing.T) {
	settings := storeSettingsFixture()
	for _, test := range []struct {
		declared string
		header   []byte
		valid    bool
	}{
		{"video/quicktime", []byte("0000ftypqt  "), true},
		{"image/avif", []byte("0000ftypavif"), true},
		{"image/avif", []byte("0000ftypavis"), true},
		{"image/avif", []byte("0000ftypisom"), false},
		{"image/avif", []byte("invalid"), false},
		{"audio/flac", []byte("fLaCdata"), true},
	} {
		if got := compatibleSignature(test.declared, test.header); got != test.valid {
			t.Errorf("%s signature: %v", test.declared, got)
		}
	}
	if mediaAlias(settings, rulesValue(settings.Rules).MIMEOggAudio) != "application/ogg" {
		t.Fatal("lost Ogg alias")
	}
	store := openTestStore(t)
	file, err := os.CreateTemp(t.TempDir(), "empty-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readMediaHeader(file); !errors.Is(err, errEmptyMediaFile) {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readMediaHeader(file); !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
	if _, err := settingsInspectMedia(settings,
		&mediaImport{file: file, source: "selected.png", item: &contentapi.Item{Kind: "image"}},
	); !errors.Is(
		err,
		os.ErrClosed,
	) {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	store.logger = slog.New(slog.NewTextHandler(&logs, nil))
	storeCloseMediaFile(t.Context(), store, file)
	storeReportCleanupFailure(store, io.ErrUnexpectedEOF)
	if logs.Len() == 0 {
		t.Fatal("file cleanup errors were not logged")
	}
	response := httptest.NewRecorder()
	ownedMediaServe((&ownedMedia{file: file}),
		response,
		httptest.NewRequest(http.MethodGet, "/media/test", nil),
	)
	if response.Code != http.StatusNotFound {
		t.Fatal("served a closed media file")
	}
	store.mediaDir = filepath.Join(t.TempDir(), "missing")
	if err := storeRemoveUnreferencedMedia(store, nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	store.mediaDir = file.Name()
	if err := storeCleanupMedia(store); err == nil {
		t.Fatal("accepted a regular file as media directory")
	}
	if err := storePrepare(store, store.path); err == nil {
		t.Fatal("startup accepted an invalid media directory")
	}
}

func TestCardsPersistWithReviewSchedule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storeClose(store) })
	folder, err := storeCreateFolder(store, &folderapi.CreateInput{Title: "Spanish"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := storeCreateCard(store, &cardapi.CreateInput{
		FolderID: folder.ID,
		Items:    textItems("  See you tomorrow? ", " Hasta mañana. "),
	})
	if err != nil {
		t.Fatal(err)
	}
	if contentapi.Items(card.Items)[0].Text != "See you tomorrow?" ||
		contentapi.Items(card.Items)[1].Text != "Hasta mañana." ||
		card.LastReviewed != 0 {
		t.Fatalf("unexpected new card: %+v", card)
	}
	initial := contentapi.Items(card.Items)[0]
	expected, err := fsrs.NewFSRS(fsrs.DefaultParam()).
		Next(fsrs.NewCard(contentapi.Schedule(initial.Schedule).LastReview), contentapi.Schedule(initial.Schedule).LastReview, fsrs.Again)
	if err != nil ||
		!reflect.DeepEqual(nativeSchedule(contentapi.Schedule(initial.Schedule)), expected.Card) ||
		!contentapi.Reviews(initial.Reviews)[0].Automatic {
		t.Fatal("initialization differs from FSRS Again")
	}
	sibling := contentapi.Items(card.Items)[1]
	now := contentapi.Schedule(initial.Schedule).Due
	for _, rating := range []int{1, 2, 3, 4} {
		before := contentapi.Schedule(contentapi.Items(card.Items)[0].Schedule)
		expected, err := fsrs.NewFSRS(fsrs.DefaultParam()).
			Next(nativeSchedule(before), now, fsrs.Rating(rating))
		if err != nil {
			t.Fatal(err)
		}
		card, err = storeReviewItemAt(store,
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
		if !reflect.DeepEqual(
			nativeSchedule(contentapi.Schedule(contentapi.Items(card.Items)[0].Schedule)),
			expected.Card,
		) ||
			!reflect.DeepEqual(contentapi.Items(card.Items)[1], sibling) ||
			card.LastReviewed != now.UnixMilli() {
			t.Fatal("rating differs from FSRS or changed sibling")
		}
		now = expected.Card.Due
	}
	if err := storeClose(store); err != nil {
		t.Fatal(err)
	}
	store, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := storeListCards(store)
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card}) {
		t.Fatalf("cards after reopen: %+v, %v", cards, err)
	}
}

// Noncanonical JSON and a sequence marker reveal accidental bucket replacement
// and rewriting of records that do not belong to an update.
func markItemBucket(t *testing.T, store *persistence, card *cardapi.Card) map[string][]byte {
	t.Helper()
	err := store.update(func(tx *bolt.Tx) error {
		bucket := testItemBucket(tx, card)
		if err := bucket.SetSequence(42); err != nil {
			return err
		}
		for index := range contentapi.Items(card.Items) {
			item := &contentapi.Items(card.Items)[index]
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

func snapshotItemRecords(t *testing.T, store *persistence, card *cardapi.Card) map[string][]byte {
	t.Helper()
	records := make(map[string][]byte)
	err := store.view(func(tx *bolt.Tx) error {
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
	item := &contentapi.Items(card.Items)[0]
	now := contentapi.Schedule(item.Schedule).Due.Add(time.Second)
	updated, err := storeReviewItemAt(store, &cardapi.ReviewInput{
		FolderID: card.FolderID, CardID: card.ID, ItemID: item.ID, Rating: 4,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	after := snapshotItemRecords(t, store, card)
	for index := range contentapi.Items(card.Items) {
		id := contentapi.Items(card.Items)[index].ID
		if bytes.Equal(before[id], after[id]) != (index != 0) {
			t.Fatalf("unexpected record change for item %d", index)
		}
	}
	if updated.LastReviewed != now.UnixMilli() ||
		updated.DueAt != contentapi.Schedule(
			contentapi.Items(card.Items)[1].Schedule,
		).Due.UnixMilli() {
		t.Fatal("returned aggregate schedule is stale")
	}
	listed, err := storeListCards(store)
	if err != nil || !reflect.DeepEqual(listed[0], updated) {
		t.Fatalf("returned card differs from stored card: %v", err)
	}
	assertReopenedCards(t, store, listed)
}

func assertReopenedCards(t *testing.T, store *persistence, expected []cardapi.Card) {
	t.Helper()
	path := store.databasePath()
	if err := storeClose(store); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storeClose(reopened) })
	actual, err := storeListCards(reopened)
	if err != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("cards changed after reopening: %v", err)
	}
}

func notificationOccurrence(card *cardapi.Card, index int) reviewapi.DueOccurrence {
	item := &contentapi.Items(card.Items)[index]
	return reviewapi.DueOccurrence{
		FolderID: card.FolderID,
		CardID:   card.ID,
		ItemID:   item.ID,
		Due:      contentapi.Schedule(item.Schedule).Due.UnixMilli(),
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
	if err := storeMarkNotified(store, batch); err != nil {
		t.Fatal(err)
	}
	for index := range cards {
		after := snapshotItemRecords(t, store, &cards[index])
		for position, item := range contentapi.Items(cards[index].Items) {
			changed := (index == 0 && position < 2) || (index == 1 && position == 0)
			if bytes.Equal(before[index][item.ID], after[item.ID]) == changed {
				t.Fatalf("unexpected change for card %d item %d", index, position)
			}
		}
	}
	err := store.update(func(tx *bolt.Tx) error {
		if err := storeMarkOccurrences(store, tx, batch); err != nil {
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
	listed, err := storeListCards(store)
	if err != nil {
		t.Fatal(err)
	}
	if contentapi.Items(listed[0].Items)[0].LastNotifiedDue != first.Due ||
		contentapi.Items(listed[0].Items)[1].LastNotifiedDue != second.Due ||
		contentapi.Items(listed[1].Items)[0].LastNotifiedDue != other.Due {
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
			err := storeMarkNotified(store, []reviewapi.DueOccurrence{
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
			if err := store.update(func(tx *bolt.Tx) error {
				return testItemBucket(
					tx,
					card,
				).Put([]byte(contentapi.Items(card.Items)[1].ID), []byte("invalid"))
			}); err != nil {
				t.Fatal(err)
			}
			before := snapshotItemRecords(t, store, card)
			var err error
			if operation == "review" {
				_, err = storeReviewItem(store, &cardapi.ReviewInput{
					FolderID: card.FolderID,
					CardID:   card.ID,
					ItemID:   contentapi.Items(card.Items)[0].ID,
					Rating:   3,
				})
			} else {
				err = storeMarkNotified(
					store,
					[]reviewapi.DueOccurrence{notificationOccurrence(card, 0)},
				)
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
	store := newPersistence(
		path,
		func() *Dependencies {
			deps := NewDependencies(
				cffbatch.New(),
				scheduling.New(new(configuration.Library(configurationFixture()))),
				storeSettingsFixture(),
			)
			deps.Logger = storageLogger(nil)
			return deps
		}(),
	)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("constructor opened database: %v", err)
	}
	checkUnavailable := func() {
		t.Helper()
		response := httptest.NewRecorder()
		storeMediaHandler(store).
			ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/media/folder/card/item", nil))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("inactive media returned %d", response.Code)
		}
	}
	checkUnavailable()
	if err := storeStart(store); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storeClose(store) })
	if err := storeStart(store); !errors.Is(err, errStoreAlreadyStarted) {
		t.Fatalf("repeated startup: %v", err)
	}
	if _, err := storeListFolders(store); err != nil {
		t.Fatal(err)
	}
	if err := storeClose(store); err != nil {
		t.Fatal(err)
	}
	checkUnavailable()
	reopened, err := openPersistenceStore(
		path,
		NewDependencies(
			cffbatch.New(),
			scheduling.New(new(configuration.Library(configurationFixture()))),
			storeSettingsFixture(),
		),
	)
	if err != nil {
		t.Fatalf("managed shutdown retained lock: %v", err)
	}
	if err := storeClose(reopened); err != nil {
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
	store := newPersistence(
		path,
		func() *Dependencies {
			deps := NewDependencies(
				cffbatch.New(),
				scheduling.New(new(configuration.Library(configurationFixture()))),
				storeSettingsFixture(),
			)
			deps.Logger = storageLogger(nil)
			return deps
		}(),
	)
	if err := storeStart(store); err == nil {
		t.Fatal("accepted unsupported schema")
	}
	if err := storeClose(store); err != nil {
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
	first, _ := storeCreateFolder(store, &folderapi.CreateInput{Title: "First"})
	second, _ := storeCreateFolder(store, &folderapi.CreateInput{Title: "Second"})
	for _, input := range []cardapi.CreateInput{
		{FolderID: first.ID, Items: textItems(" ", "answer")},
		{FolderID: first.ID, Items: textItems("question", " \n")},
		{FolderID: "missing", Items: textItems("question", "answer")},
	} {
		if _, err := storeCreateCard(store, &input); err == nil {
			t.Fatalf("accepted invalid card: %+v", input)
		}
	}
	cards, err := storeListCards(store)
	if err != nil || cards == nil || len(cards) != 0 {
		t.Fatalf("failed save wrote cards: %+v, %v", cards, err)
	}
	card, err := storeCreateCard(store,
		&cardapi.CreateInput{FolderID: first.ID, Items: textItems("Question", "Answer")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storeReviewItem(store,
		&cardapi.ReviewInput{
			FolderID: second.ID,
			CardID:   card.ID,
			ItemID:   contentapi.Items(card.Items)[0].ID,
			Rating:   3,
		},
	); err == nil {
		t.Fatal("reviewed a card through the wrong folder")
	}
	if _, err := storeReviewItem(store,
		&cardapi.ReviewInput{
			FolderID: first.ID,
			CardID:   "missing",
			ItemID:   contentapi.Items(card.Items)[0].ID,
			Rating:   3,
		},
	); err == nil {
		t.Fatal("reviewed a missing card")
	}
	cards, err = storeListCards(store)
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card}) {
		t.Fatalf("failed review changed cards: %+v, %v", cards, err)
	}
	if err := storeClose(store); err != nil {
		t.Fatal(err)
	}
	if _, err := storeListCards(store); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("closed list: %v", err)
	}
	if _, err := storeCreateCard(store,
		&cardapi.CreateInput{FolderID: first.ID, Items: textItems("Question", "Answer")},
	); !errors.Is(
		err,
		bolt.ErrDatabaseNotOpen,
	) {
		t.Fatalf("closed create: %v", err)
	}
	if _, err := storeReviewItem(store,
		&cardapi.ReviewInput{
			FolderID: first.ID,
			CardID:   card.ID,
			ItemID:   contentapi.Items(card.Items)[0].ID,
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
	drafts := make([]contentapi.Draft, 0, len(contentapi.Items(card.Items)))
	for _, item := range contentapi.Items(card.Items) {
		drafts = append(drafts, contentapi.Draft{ID: item.ID, Kind: item.Kind, Text: item.Text})
	}
	return drafts
}

func TestMixedContentPersistenceEditingAndMediaCopies(t *testing.T) {
	s := openTestStore(t)
	folder, _ := storeCreateFolder(s, &folderapi.CreateInput{Title: "Mixed"})
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
	card, err := storeCreateCard(s, &cardapi.CreateInput{FolderID: folder.ID, Items: drafts})
	if err != nil {
		t.Fatal(err)
	}
	if len(contentapi.Items(card.Items)) != 16 {
		t.Fatalf("items: %+v", contentapi.Items(card.Items))
	}
	for i, item := range contentapi.Items(card.Items) {
		if item.Position != i || item.FolderID != folder.ID || item.CardID != card.ID {
			t.Fatalf("relation/order: %+v", item)
		}
		if item.Kind != "text" {
			data, err := os.ReadFile(storeMediaPath(s, item.RelativePath))
			if err != nil || int64(len(data)) != item.ByteSize {
				t.Fatalf("managed copy: %v", err)
			}
			if err := os.Remove(drafts[i].SourcePath); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := s.view(func(tx *bolt.Tx) error {
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
	card, err = storeReviewItem(s,
		&cardapi.ReviewInput{
			FolderID: folder.ID,
			CardID:   card.ID,
			ItemID:   contentapi.Items(card.Items)[0].ID,
			Rating:   3,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	due := contentapi.Schedule(contentapi.Items(card.Items)[0].Schedule).Due.UnixMilli()
	for _, item := range contentapi.Items(card.Items)[2:] {
		if value := contentapi.Schedule(item.Schedule).Due.UnixMilli(); value < due {
			due = value
		}
	}
	reviewed := card.LastReviewed
	edit := itemDrafts(card)
	removed := contentapi.Items(card.Items)[1]
	edit = append(edit[:1], edit[2:]...)
	edit[0], edit[1] = edit[1], edit[0]
	edit[1].Text = "Updated text"
	updated, err := storeUpdateCard(s,
		&cardapi.UpdateInput{FolderID: folder.ID, CardID: card.ID, Items: edit},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DueAt != due || updated.LastReviewed != reviewed ||
		updated.CreatedAt != card.CreatedAt {
		t.Fatal("edit changed schedule")
	}
	if contentapi.Items(updated.Items)[0].ID != edit[0].ID ||
		contentapi.Items(updated.Items)[1].Text != "Updated text" {
		t.Fatal("edit/reorder lost")
	}
	if _, err := os.Stat(storeMediaPath(s, removed.RelativePath)); !os.IsNotExist(err) {
		t.Fatal("removed media not cleaned")
	}
	path := s.databasePath()
	if err := storeClose(s); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storeClose(reopened)
	cards, err := storeListCards(reopened)
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{updated}) {
		t.Fatalf("reopen mismatch: %v", err)
	}
}

func TestContentValidationAndRollback(t *testing.T) {
	s := openTestStore(t)
	folder, _ := storeCreateFolder(s, &folderapi.CreateInput{Title: "First"})
	other, _ := storeCreateFolder(s, &folderapi.CreateInput{Title: "Second"})
	card, err := storeCreateCard(s,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("Prompt", "Answer")},
	)
	if err != nil {
		t.Fatal(err)
	}
	otherCard, err := storeCreateCard(s,
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
		{{ID: contentapi.Items(card.Items)[0].ID, Kind: "image", SourcePath: "anything"}},
	}
	for _, drafts := range invalid {
		if _, err := storeUpdateCard(s,
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
	if _, err := storeUpdateCard(s,
		&cardapi.UpdateInput{FolderID: folder.ID, CardID: card.ID, Items: failed},
	); err == nil {
		t.Fatal("accepted missing import")
	}
	entries, _ := os.ReadDir(s.mediaDir)
	if len(entries) != 0 {
		t.Fatal("failed import left media files")
	}
	// Force a failure inside the write transaction after a successful copy.
	if err := s.update(
		func(tx *bolt.Tx) error { bucket, _ := cardBucket(tx, folder.ID); return bucket.SetSequence(0) },
	); err != nil {
		t.Fatal(err)
	}
	if _, err := storeCreateCard(s, &cardapi.CreateInput{
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
	cards, err := storeListCards(s)
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card, otherCard}) {
		t.Fatalf("failed edit changed data: %+v, %v", cards, err)
	}
}

func TestMediaOnlyCardsStreamingAndCleanup(t *testing.T) {
	s := openTestStore(t)
	folder, _ := storeCreateFolder(s, &folderapi.CreateInput{Title: "Media"})
	other, _ := storeCreateFolder(s, &folderapi.CreateInput{Title: "Other"})
	image, imageData := mediaFixture(t, ".png")
	audio, _ := mediaFixture(t, ".wav")
	card, err := storeCreateCard(s, &cardapi.CreateInput{
		FolderID: folder.ID,
		Items: []contentapi.Draft{
			{Kind: "image", SourcePath: image},
			{Kind: "audio", SourcePath: audio},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	copyCard, err := storeCreateCard(s, &cardapi.CreateInput{
		FolderID: folder.ID,
		Items: []contentapi.Draft{
			{Kind: "image", SourcePath: image},
			{Kind: "audio", SourcePath: audio},
		},
	})
	if err != nil ||
		contentapi.Items(copyCard.Items)[0].RelativePath == contentapi.Items(card.Items)[0].RelativePath {
		t.Fatal("media was shared between cards")
	}
	url := "/media/" + folder.ID + "/" + card.ID + "/" + contentapi.Items(card.Items)[0].ID
	handler := storeMediaHandler(s)
	for _, test := range []struct {
		method, url, byteRange string
		status                 int
		body                   []byte
	}{
		{"GET", url, "", http.StatusOK, imageData},
		{"GET", url, "bytes=2-7", http.StatusPartialContent, imageData[2:8]},
		{"HEAD", url, "", http.StatusOK, nil},
		{"GET", url, "bytes=99999-", http.StatusRequestedRangeNotSatisfiable, nil},
		{"GET", "/media/" + other.ID + "/" + card.ID + "/" + contentapi.Items(card.Items)[0].ID, "", http.StatusNotFound, nil},
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
	if err := os.Remove(
		storeMediaPath(s, contentapi.Items(card.Items)[0].RelativePath),
	); err != nil {
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
	path := s.databasePath()
	if err := storeClose(s); err != nil {
		t.Fatal(err)
	}
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storeClose(s)
	for _, name := range []string{orphan, ".import-abandoned"} {
		if _, err := os.Stat(filepath.Join(s.mediaDir, name)); !os.IsNotExist(err) {
			t.Fatal("startup orphan cleanup failed")
		}
	}
	if _, err := os.Stat(filepath.Join(s.mediaDir, "unrelated.txt")); err != nil {
		t.Fatal("removed unmanaged file")
	}
	if _, err := os.Stat(
		storeMediaPath(s, contentapi.Items(copyCard.Items)[0].RelativePath),
	); err != nil {
		t.Fatal("removed referenced media")
	}
	if cards, err := storeListCards(s); err != nil || len(cards) != 2 {
		t.Fatalf("missing file prevented loading other content: %v", err)
	}
}

func TestMediaTypeValidation(t *testing.T) {
	s := openTestStore(t)
	folder, _ := storeCreateFolder(s, &folderapi.CreateInput{Title: "Media"})
	path := filepath.Join(t.TempDir(), "fake.png")
	if err := os.WriteFile(path, []byte("<html>not an image</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	image, _ := mediaFixture(t, ".png")
	for _, draft := range []contentapi.Draft{{Kind: "image", SourcePath: path}, {Kind: "video", SourcePath: image}, {Kind: "image", SourcePath: filepath.Dir(image)}} {
		if _, err := storeCreateCard(s, &cardapi.CreateInput{
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
	if _, err := storeCreateCard(s, &cardapi.CreateInput{
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
	folder, _ := storeCreateFolder(s, &folderapi.CreateInput{Title: "Concurrent"})
	card, err := storeCreateCard(s,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("prompt", "answer")},
	)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := storeReviewItem(s,
				&cardapi.ReviewInput{
					FolderID: folder.ID,
					CardID:   card.ID,
					ItemID:   contentapi.Items(card.Items)[0].ID,
					Rating:   3,
				},
			); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() {
			if _, err := storeUpdateCard(s, &cardapi.UpdateInput{
				FolderID: folder.ID,
				CardID:   card.ID,
				Items:    itemDrafts(card),
			}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	cards, err := storeListCards(s)
	if err != nil || contentapi.Schedule(contentapi.Items(cards[0].Items)[0].Schedule).Reps != 9 ||
		cards[0].LastReviewed == 0 ||
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
	cards, err := storeListCards(s)
	if err != nil || len(cards) != 1 {
		t.Fatalf("migration: %v", err)
	}
	card := cards[0]
	if card.CreatedAt != 1000 || card.DueAt <= 2000 || card.LastReviewed != 0 ||
		len(contentapi.Items(card.Items)) != 2 ||
		contentapi.Items(card.Items)[0].Text != "Old prompt" ||
		contentapi.Items(card.Items)[1].Text != "Old answer" {
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
	_ = storeClose(s)
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storeClose(s)
	again, err := storeListCards(s)
	if err != nil || !reflect.DeepEqual(cards, again) {
		t.Fatal("migration repeated or duplicated items")
	}
	backups, _ = filepath.Glob(path + ".v1-*.bak")
	if len(backups) != 1 {
		t.Fatal("made another backup after migration")
	}
	next, err := storeCreateCard(s,
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
		_ = storeClose(s)
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
	folder, _ := storeCreateFolder(s, &folderapi.CreateInput{Title: "Information"})
	card, err := storeCreateCard(s,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("A fact worth remembering")},
	)
	if err != nil || len(contentapi.Items(card.Items)) != 1 {
		t.Fatalf("single text card: %+v, %v", card, err)
	}
	image, _ := mediaFixture(t, ".png")
	updated, err := storeUpdateCard(s, &cardapi.UpdateInput{
		FolderID: folder.ID,
		CardID:   card.ID,
		Items:    []contentapi.Draft{{Kind: "image", SourcePath: image}},
	})
	if err != nil || len(contentapi.Items(updated.Items)) != 1 ||
		contentapi.Items(updated.Items)[0].Kind != "image" ||
		contentapi.Items(updated.Items)[0].Position != 0 {
		t.Fatalf("single media card: %+v, %v", updated, err)
	}
	if _, err := storeUpdateCard(s,
		&cardapi.UpdateInput{FolderID: folder.ID, CardID: card.ID, Items: nil},
	); err == nil {
		t.Fatal("accepted an empty information card")
	}
	cards, err := storeListCards(s)
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
			folder, _ := storeCreateFolder(s, &folderapi.CreateInput{Title: "Existing content"})
			image, _ := mediaFixture(t, ".png")
			card, err := storeCreateCard(s, &cardapi.CreateInput{
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
			card, err = storeReviewItem(s,
				&cardapi.ReviewInput{
					FolderID: folder.ID,
					CardID:   card.ID,
					ItemID:   contentapi.Items(card.Items)[0].ID,
					Rating:   3,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			originals := map[string][]byte{}
			err = s.update(func(tx *bolt.Tx) error {
				if err := tx.Bucket([]byte(metaBucket)).
					Put([]byte(schemaKey), []byte("2")); err != nil {
					return err
				}
				bucket := tx.Bucket([]byte(foldersBucket)).
					Bucket([]byte(folder.ID)).
					Bucket([]byte(itemsBucket)).
					Bucket([]byte(card.ID))
				for i, item := range contentapi.Items(card.Items) {
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
			if err := storeClose(s); err != nil {
				t.Fatal(err)
			}
			s, err = openStore(path)
			if malformed {
				if err == nil {
					_ = storeClose(s)
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
			cards, err := storeListCards(s)
			if err != nil || len(cards) != 1 {
				t.Fatalf("migrated cards: %+v, %v", cards, err)
			}
			want := card
			want.Items = []contentapi.Item{
				contentapi.Items(card.Items)[1],
				contentapi.Items(card.Items)[0],
				contentapi.Items(card.Items)[2],
			}
			for i := range contentapi.Items(want.Items) {
				contentapi.Items(want.Items)[i].Position = i
				contentapi.Items(want.Items)[i].Schedule = contentapi.Schedule(
					contentapi.Items(cards[0].Items)[i].Schedule,
				)
				contentapi.Items(want.Items)[i].Reviews = contentapi.Reviews(
					contentapi.Items(cards[0].Items)[i].Reviews,
				)
				if len(contentapi.Reviews(contentapi.Items(want.Items)[i].Reviews)) != 1 ||
					!contentapi.Reviews(contentapi.Items(want.Items)[i].Reviews)[0].Automatic {
					t.Fatal("migration did not initialize Again")
				}
			}
			want.DueAt = cards[0].DueAt
			want.LastReviewed = 0
			if !reflect.DeepEqual(cards[0], want) {
				t.Fatalf("content/history lost: got %+v, want %+v", cards[0], want)
			}
			if _, err := os.Stat(
				storeMediaPath(s, contentapi.Items(want.Items)[0].RelativePath),
			); err != nil {
				t.Fatal("migration lost media file")
			}
			err = s.view(func(tx *bolt.Tx) error {
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
			_ = storeClose(s)
			s, err = openStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer storeClose(s)
			again, err := storeListCards(s)
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
	defer storeClose(s)
	first, err := storeCreateFolder(s, &folderapi.CreateInput{Title: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := storeCreateFolder(s, &folderapi.CreateInput{Title: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	create := func(folder string) cardapi.Card {
		t.Helper()
		card, err := storeCreateCard(s,
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
	if err := storeDeleteCard(s, first.ID, removed.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.view(func(tx *bolt.Tx) error {
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
	cards, err := storeListCards(s)
	if err != nil || len(cards) != 2 {
		t.Fatalf("after card deletion: %v, %v", cards, err)
	}
	if err := storeDeleteCard(s, first.ID, removed.ID); err == nil {
		t.Fatal("expected missing card error")
	}
	if err := storeDeleteFolder(s, first.ID); err != nil {
		t.Fatal(err)
	}
	cards, err = storeListCards(s)
	if err != nil || len(cards) != 1 || cards[0].FolderID != survivor.FolderID ||
		cards[0].ID != survivor.ID {
		t.Fatalf("after folder deletion: %v, %v", cards, err)
	}
	folders, err := storeListFolders(s)
	if err != nil || len(folders) != 1 || folders[0].ID != second.ID {
		t.Fatalf("remaining folders: %v, %v", folders, err)
	}
	if err := storeDeleteFolder(s, first.ID); err == nil {
		t.Fatal("expected missing folder error")
	}
}

func openTestStore(t *testing.T) *persistence {
	t.Helper()
	store, err := openStore(filepath.Join(t.TempDir(), "raven.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storeClose(store) })
	return store
}

func TestListingRetriesUseFreshResultsAndDoNotReplayWrites(t *testing.T) {
	storage := openTestStore(t)
	storage.reads = resilience.NewReads(t.Context(), nil).Do
	view := storage.view
	calls := 0
	storage.view = func(callback func(*bolt.Tx) error) error {
		calls++
		err := view(callback)
		if calls == 1 && err == nil {
			return syscall.EAGAIN
		}
		return err
	}
	value, err := (&Store{state: storage}).ListFolders()
	if err != nil || value != "[]" || calls != 2 {
		t.Fatal(value, calls, err)
	}
	writes := 0
	update := storage.update
	storage.update = func(callback func(*bolt.Tx) error) error { writes++; return syscall.EAGAIN }
	if _, err := (&Store{state: storage}).CreateFolder(
		`{"title":"test"}`,
	); err == nil ||
		writes != 1 {
		t.Fatal(writes, err)
	}
	storage.update = update
}

func TestLifecycleStartupContextsAndLegacyResources(t *testing.T) {
	deps := NewDependencies(
		cffbatch.New(),
		scheduling.New(new(configuration.Library(configurationFixture()))),
		storeSettingsFixture(),
	)
	store := New(filepath.Join(t.TempDir(), "raven.db"), deps)
	if err := StartContext(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	if err := StartContext(t.Context(), store); !errors.Is(err, errStoreAlreadyStarted) {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	store = New(filepath.Join(t.TempDir(), "cancel.db"), deps)
	store.state.initialization = databaseInitializationCallbacks(resilience.NewInitialization(
		t.Context(),
		func(ctx context.Context) (*bolt.DB, error) { <-ctx.Done(); return nil, ctx.Err() },
		&resilience.Options[*bolt.DB]{Release: (*bolt.DB).Close, Transient: resilience.Transient},
	))
	if err := StartContext(ctx, store); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "late.db")
	if db, err := acquireDatabase(ctx, path); db != nil || !errors.Is(err, context.Canceled) {
		t.Fatal(db, err)
	}
	if _, err := databaseForStartup(t.Context(), &persistence{path: t.TempDir()}); err == nil {
		t.Fatal("directory accepted as database")
	}
	legacy := newStore(nil, path, deps)
	if err := closePersistence(legacy); err != nil {
		t.Fatal(err)
	}
	db, err := databaseForStartup(t.Context(), legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if listingContext(legacy).Err() != nil {
		t.Fatal("unexpected listing context")
	}
	value, err := readListing(
		t.Context(),
		legacy,
		func(*persistence) (string, error) { return "[]", nil },
	)
	if err != nil || value != "[]" {
		t.Fatal(value, err)
	}
	legacy.reads = resilience.NewReads(t.Context(), nil).Do
	_, err = readListing(
		t.Context(),
		legacy,
		func(*persistence) (string, error) { return "partial", os.ErrPermission },
	)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatal(err)
	}
	legacy.closeDatabase = func() error { return os.ErrPermission }
	if err := closePersistence(legacy); !errors.Is(err, os.ErrPermission) {
		t.Fatal(err)
	}
	legacy.closeDatabase = func() error { return nil }
	if err := closePersistence(legacy); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyDatabaseAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven", "raven.db")
	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storeClose(store)
	folders, err := storeListFolders(store)
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

func TestDatabasePathInspectionErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink loops are not consistently available on Windows")
	}

	path := filepath.Join(t.TempDir(), "raven.db")
	if err := os.Symlink(path, path); err != nil {
		t.Skipf("create symlink loop: %v", err)
	}

	if _, err := existingDatabaseFile(path); err == nil {
		t.Fatal("expected inspecting a symlink loop to fail")
	}
	if err := validateExistingDatabase(
		path,
	); err == nil ||
		!strings.Contains(err.Error(), "inspect existing database") {
		t.Fatalf("expected wrapped inspection error, got %v", err)
	}
}

func TestOpenDatabaseReportsOpenFailure(t *testing.T) {
	want := errors.New("open failed")
	path := filepath.Join(t.TempDir(), "raven.db")
	_, err := openDatabase(path, func(string, os.FileMode, *bolt.Options) (*bolt.DB, error) {
		return nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected wrapped open error, got %v", err)
	}
}

func TestReadOnlyDatabaseReportsCloseFailure(t *testing.T) {
	want := errors.New("close failed")
	err := validateDatabaseReadOnly(
		"unused.db",
		func(string, os.FileMode, *bolt.Options) (func() error, error) {
			return func() error { return want }, nil
		},
	)
	if !errors.Is(err, want) {
		t.Fatalf("expected wrapped close error, got %v", err)
	}
}

func TestFoldersPersistInCreationOrderWithCardsBuckets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storeClose(store) })
	want := make([]folderapi.Folder, 0)
	for i := range 12 {
		folder, err := storeCreateFolder(store, &folderapi.CreateInput{
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
	if err := storeClose(store); err != nil {
		t.Fatal(err)
	}
	store, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := storeListFolders(store)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("reopened folders: got=%+v, want=%+v, error=%v", got, want, err)
	}
	err = store.view(func(tx *bolt.Tx) error {
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
	next, err := storeCreateFolder(store, &folderapi.CreateInput{Title: "After restart"})
	if err != nil || next.ID != fmt.Sprintf("%020d", len(want)+1) || next.Tags == nil {
		t.Fatalf("sequence/tags after reopen: %+v, error=%v", next, err)
	}
}

func TestValidationDoesNotWrite(t *testing.T) {
	store := openTestStore(t)
	for _, title := range []string{"", " \n\t ", "\u3000"} {
		if _, err := storeCreateFolder(store, &folderapi.CreateInput{Title: title}); err == nil {
			t.Errorf("accepted empty title %q", title)
		}
	}
	folders, err := storeListFolders(store)
	if err != nil || len(folders) != 0 {
		t.Fatalf("validation wrote folders: %v, error=%v", folders, err)
	}
	folder, err := storeCreateFolder(store, &folderapi.CreateInput{Title: "Valid"})
	if err != nil || folder.ID != "00000000000000000001" || folder.Tags == nil {
		t.Fatalf("invalid sequence or null tags: %+v, error=%v", folder, err)
	}
}

func TestConcurrentFolderCreation(t *testing.T) {
	store := openTestStore(t)
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			if _, err := storeCreateFolder(store,
				&folderapi.CreateInput{Title: fmt.Sprintf("folderapi.Folder %d", i)},
			); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	folders, err := storeListFolders(store)
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
	original, err := storeCreateFolder(store, &folderapi.CreateInput{Title: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	// Force a collision after NextSequence so the failed transaction must roll it back.
	if err := store.update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(foldersBucket)).SetSequence(0)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := storeCreateFolder(store,
		&folderapi.CreateInput{Title: "Must not overwrite"},
	); !errors.Is(
		err,
		bolt.ErrBucketExists,
	) {
		t.Fatalf("expected bucket collision, got %v", err)
	}
	if err := store.view(func(tx *bolt.Tx) error {
		if sequence := tx.Bucket([]byte(foldersBucket)).Sequence(); sequence != 0 {
			return fmt.Errorf("failed transaction advanced sequence to %d", sequence)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	folders, err := storeListFolders(store)
	if err != nil || !reflect.DeepEqual(folders, []folderapi.Folder{original}) {
		t.Fatalf("failed transaction changed folders: %+v, error=%v", folders, err)
	}
}

func TestDatabaseLockTimesOut(t *testing.T) {
	store := openTestStore(t)
	start := time.Now()
	other, err := openStore(store.databasePath())
	if other != nil {
		_ = storeClose(other)
	}
	if !errors.Is(err, bolt.ErrTimeout) ||
		!strings.Contains(err.Error(), strconv.Quote(store.databasePath())) {
		t.Fatalf("expected lock timeout with path, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond || elapsed > 15*time.Second {
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
		_ = storeClose(store)
	}
	if err == nil || !strings.Contains(err.Error(), strconv.Quote(path)) {
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
		if _, err := storeCreateFolder(store, &folderapi.CreateInput{Title: "Ideas"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(foldersBucket)).
			Bucket([]byte("00000000000000000002")).
			Put([]byte(metadataKey), []byte("{broken"))
	}); err != nil {
		t.Fatal(err)
	}
	if folders, err := storeListFolders(store); err == nil || folders != nil {
		t.Fatalf("expected error without partial results: %+v, error=%v", folders, err)
	}
}

func TestClosedStoreReturnsErrors(t *testing.T) {
	store := openTestStore(t)
	if err := storeClose(store); err != nil {
		t.Fatal(err)
	}
	if _, err := storeListFolders(store); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("expected closed database error, got %v", err)
	}
	if _, err := storeCreateFolder(store, &folderapi.CreateInput{Title: "Ideas"}); !errors.Is(
		err,
		bolt.ErrDatabaseNotOpen,
	) {
		t.Fatalf("expected closed database error, got %v", err)
	}
}

func TestItemReviewValidationAndScheduleEdits(t *testing.T) {
	s := openTestStore(t)
	folder, _ := storeCreateFolder(s, &folderapi.CreateInput{Title: "Learning"})
	card, err := storeCreateCard(s,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("one", "two")},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []struct {
		item   string
		rating int
	}{{contentapi.Items(card.Items)[0].ID, 0}, {contentapi.Items(card.Items)[0].ID, 5}, {"missing", 3}, {contentapi.Items(card.Items)[1].ID, -1}} {
		if _, err := storeReviewItem(s,
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
	cards, _ := storeListCards(s)
	if !reflect.DeepEqual(cards[0], card) {
		t.Fatal("invalid review mutated data")
	}
	// An invalid FSRS time must roll back both the schedule and its review log.
	if _, err := storeReviewItemAt(s,
		&cardapi.ReviewInput{
			FolderID: folder.ID,
			CardID:   card.ID,
			ItemID:   contentapi.Items(card.Items)[0].ID,
			Rating:   3,
		},
		contentapi.Schedule(contentapi.Items(card.Items)[0].Schedule).LastReview.Add(-time.Second),
	); err == nil {
		t.Fatal("accepted review before last review")
	}
	cards, _ = storeListCards(s)
	if !reflect.DeepEqual(cards[0], card) {
		t.Fatal("failed FSRS operation mutated data")
	}
	updated, err := storeUpdateCard(s,
		&cardapi.UpdateInput{FolderID: folder.ID, CardID: card.ID, Items: []contentapi.Draft{
			{
				ID:   contentapi.Items(card.Items)[1].ID,
				Kind: "text",
				Text: "edited",
			},
			{Kind: "text", Text: "new"},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(
		contentapi.Schedule(contentapi.Items(updated.Items)[0].Schedule),
		contentapi.Schedule(contentapi.Items(card.Items)[1].Schedule),
	) ||
		!reflect.DeepEqual(
			contentapi.Reviews(contentapi.Items(updated.Items)[0].Reviews),
			contentapi.Reviews(contentapi.Items(card.Items)[1].Reviews),
		) {
		t.Fatal("edit reset schedule")
	}
	if contentapi.Schedule(contentapi.Items(updated.Items)[1].Schedule).Reps != 1 ||
		!contentapi.Reviews(contentapi.Items(updated.Items)[1].Reviews)[0].Automatic {
		t.Fatal("new item did not start Again")
	}
	if _, err := storeReviewItem(s,
		&cardapi.ReviewInput{
			FolderID: folder.ID,
			CardID:   card.ID,
			ItemID:   contentapi.Items(card.Items)[0].ID,
			Rating:   3,
		},
	); err == nil {
		t.Fatal("removed item remains reviewable")
	}
}

func TestNotificationAcknowledgmentPersistsAndRejectsStaleSchedules(t *testing.T) {
	s := openTestStore(t)
	folder, _ := storeCreateFolder(s, &folderapi.CreateInput{Title: "Reminders"})
	card, _ := storeCreateCard(s,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("one", "two")},
	)
	first := contentapi.Items(card.Items)[0]
	occurrence := reviewapi.DueOccurrence{
		FolderID: folder.ID,
		CardID:   card.ID,
		ItemID:   first.ID,
		Due:      contentapi.Schedule(first.Schedule).Due.UnixMilli(),
	}
	if err := storeMarkNotified(s, []reviewapi.DueOccurrence{occurrence}); err != nil {
		t.Fatal(err)
	}
	card, err := storeReviewItem(s,
		&cardapi.ReviewInput{FolderID: folder.ID, CardID: card.ID, ItemID: first.ID, Rating: 4},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := storeMarkNotified(s, []reviewapi.DueOccurrence{occurrence}); err != nil {
		t.Fatal(err)
	}
	cards, _ := storeListCards(s)
	if contentapi.Items(cards[0].Items)[0].LastNotifiedDue != occurrence.Due ||
		contentapi.Items(cards[0].Items)[1].LastNotifiedDue != 0 ||
		contentapi.Items(cards[0].Items)[0].LastNotifiedDue == contentapi.Schedule(
			contentapi.Items(card.Items)[0].Schedule,
		).Due.UnixMilli() {
		t.Fatal("stale acknowledgment suppressed new review")
	}
	claimed, err := storeClaimNotificationPrompt(s)
	if err != nil || !claimed {
		t.Fatal("first authorization not claimed")
	}
	path := s.databasePath()
	_ = storeClose(s)
	reopened, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storeClose(reopened)
	if claimed, err := storeClaimNotificationPrompt(reopened); err != nil || claimed {
		t.Fatal("authorization prompted twice")
	}
	persisted, err := storeListCards(reopened)
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
	folder, _ := storeCreateFolder(s, &folderapi.CreateInput{Title: "V3"})
	image, _ := mediaFixture(t, ".png")
	card, err := storeCreateCard(s, &cardapi.CreateInput{
		FolderID: folder.ID,
		Items: []contentapi.Draft{
			{Kind: "text", Text: "text"},
			{Kind: "image", SourcePath: image},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = s.update(func(tx *bolt.Tx) error {
		if err := tx.Bucket([]byte(metaBucket)).Put([]byte(schemaKey), []byte("3")); err != nil {
			return err
		}
		bucket := tx.Bucket([]byte(foldersBucket)).
			Bucket([]byte(folder.ID)).
			Bucket([]byte(itemsBucket)).
			Bucket([]byte(card.ID))
		for _, item := range contentapi.Items(card.Items) {
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
	_ = storeClose(s)
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := storeListCards(s)
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range contentapi.Items(cards[0].Items) {
		if item.ID != contentapi.Items(card.Items)[i].ID ||
			item.RelativePath != contentapi.Items(card.Items)[i].RelativePath ||
			contentapi.Schedule(item.Schedule).Reps != 1 ||
			!contentapi.Reviews(item.Reviews)[0].Automatic {
			t.Fatal("migration lost content or failed to initialize Again")
		}
	}
	backups, _ := filepath.Glob(path + ".v3-*.bak")
	if len(backups) != 1 {
		t.Fatal("missing migration backup")
	}
	_ = storeClose(s)
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storeClose(s)
	again, err := storeListCards(s)
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
	t.Cleanup(func() { _ = storeClose(s) })
	folder, err := storeCreateFolder(s,
		&folderapi.CreateInput{Title: "Existing folder", Tags: []string{"retained"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err := storeCreateCard(s,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("Existing memory")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if first, err := storeClaimAutostartInitialization(s); err != nil || !first {
		t.Fatalf("first claim=%v error=%v", first, err)
	}
	if err := storeClose(s); err != nil {
		t.Fatal(err)
	}
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if first, err := storeClaimAutostartInitialization(s); err != nil || first {
		t.Fatalf("reopened claim=%v error=%v", first, err)
	}
	folders, err := storeListFolders(s)
	if err != nil || !reflect.DeepEqual(folders, []folderapi.Folder{folder}) {
		t.Fatalf("folders changed: %+v, %v", folders, err)
	}
	cards, err := storeListCards(s)
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card}) {
		t.Fatalf("cards changed: %+v, %v", cards, err)
	}
	if err := s.view(func(tx *bolt.Tx) error {
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
			first, err := storeClaimAutostartInitialization(s)
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
	_ = storeClose(s)
	if first, err := storeClaimAutostartInitialization(s); err == nil || first {
		t.Fatalf("closed database claim=%v error=%v", first, err)
	}
}

func openStore(path string) (*persistence, error) {
	return openPersistenceStore(
		path,
		NewDependencies(
			cffbatch.New(),
			scheduling.New(new(configuration.Library(configurationFixture()))),
			storeSettingsFixture(),
		),
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
	folder, err := storeCreateFolder(storage, &folderapi.CreateInput{Title: "Existing"})
	if err != nil {
		t.Fatal(err)
	}
	media, contents := mediaFixture(t, ".png")
	card, err := storeCreateCard(storage,
		&cardapi.CreateInput{FolderID: folder.ID, Items: []contentapi.Draft{
			{Kind: "text", Text: "Keep this schedule"}, {Kind: "image", SourcePath: media},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err = storeReviewItem(storage,
		&cardapi.ReviewInput{
			FolderID: folder.ID,
			CardID:   card.ID,
			ItemID:   contentapi.Items(card.Items)[0].ID,
			Rating:   3,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = storeMarkNotified(storage,
		[]reviewapi.DueOccurrence{
			{
				FolderID: folder.ID,
				CardID:   card.ID,
				ItemID:   contentapi.Items(card.Items)[0].ID,
				Due: contentapi.Schedule(
					contentapi.Items(card.Items)[0].Schedule,
				).Due.UnixMilli(),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := storeListCards(storage)
	if err != nil {
		t.Fatal(err)
	}
	want := cards[0]
	claimed, err := storeClaimAutostartInitialization(storage)
	if err != nil || !claimed {
		t.Fatalf("claim startup: %v %v", claimed, err)
	}
	claimed, err = storeClaimNotificationPrompt(storage)
	if err != nil || !claimed {
		t.Fatalf("claim prompt: %v %v", claimed, err)
	}
	err = storage.update(func(tx *bolt.Tx) error {
		folderBucket := tx.Bucket([]byte(foldersBucket)).Bucket([]byte(folder.ID))
		cardsBucket := folderBucket.Bucket([]byte(cardsBucket))
		if err := cardsBucket.Put(
			[]byte(card.ID),
			v4JSON(t, cardsBucket.Get([]byte(card.ID))),
		); err != nil {
			return err
		}
		items := folderBucket.Bucket([]byte(itemsBucket)).Bucket([]byte(card.ID))
		for _, item := range contentapi.Items(want.Items) {
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
	if err := storeClose(storage); err != nil {
		t.Fatal(err)
	}
	for attempt := range 2 {
		reopened, err := openStore(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := storeListCards(reopened)
		if err != nil || !reflect.DeepEqual(got, []cardapi.Card{want}) {
			t.Fatalf("lost records: %+v %v", got, err)
		}
		if claimed, err := storeClaimAutostartInitialization(reopened); err != nil || claimed {
			t.Fatalf("lost startup marker: %v %v", claimed, err)
		}
		if claimed, err := storeClaimNotificationPrompt(reopened); err != nil || claimed {
			t.Fatalf("lost prompt marker: %v %v", claimed, err)
		}
		data, err := os.ReadFile(
			storeMediaPath(reopened, contentapi.Items(want.Items)[1].RelativePath),
		)
		if err != nil || !bytes.Equal(data, contents) {
			t.Fatalf("lost media: %v", err)
		}
		err = reopened.view(func(tx *bolt.Tx) error {
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
		if err := storeClose(reopened); err != nil {
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
	folder, err := storeCreateFolder(storage, &folderapi.CreateInput{Title: "Existing"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := storeCreateCard(storage,
		&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("Keep me")},
	)
	if err != nil {
		t.Fatal(err)
	}
	var original []byte
	err = storage.update(func(tx *bolt.Tx) error {
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
		if err := items.Put(
			[]byte(contentapi.Items(card.Items)[0].ID),
			[]byte(`{"invalid":`),
		); err != nil {
			return err
		}
		return tx.Bucket([]byte(metaBucket)).Put([]byte(schemaKey), []byte("4"))
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := storeClose(storage); err != nil {
		t.Fatal(err)
	}
	if failed, err := openStore(path); err == nil {
		storeClose(failed)
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

func TestMediaCopyPreservesCopyAndCloseFailures(t *testing.T) {
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
		t.Fatalf("expected copy and close failures, got %v", failures)
	}
	for index, path := range []string{source.Name(), target.Name()} {
		var failure *os.PathError
		if !errors.As(failures[index], &failure) ||
			(failure.Path != path && (index != 0 || failure.Path != target.Name())) ||
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
	if err = settingsInspectImportedMedia(
		storeSettingsFixture(),
		&mediaImport{file: source, source: path, item: &item},
	); err != nil {
		t.Fatal(err)
	}
	if item.ByteSize != int64(len(content)) || item.MIMEType != "image/png" {
		t.Fatalf("unexpected imported metadata: %+v", item)
	}
	item.ByteSize++
	if _, err = storePersistMedia(store,
		&mediaImport{file: source, source: path, item: &item},
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
	if err = settingsInspectImportedMedia(storeSettingsFixture(),
		&mediaImport{file: closed, source: path, item: &item},
	); err == nil || !strings.Contains(err.Error(), "inspect media file") {
		t.Fatalf("expected stat failure, got %v", err)
	}
	if _, err = storeCommitMedia(store,
		filepath.Join(t.TempDir(), "missing"),
		storeMediaPath(store, item.RelativePath),
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
		{
			"missing due",
			func(item *contentapi.Item) {
				schedule := contentapi.Schedule(item.Schedule)
				schedule.Due = time.Time{}
				item.Schedule = schedule
			},
		},
		{
			"missing review time",
			func(item *contentapi.Item) {
				schedule := contentapi.Schedule(item.Schedule)
				schedule.LastReview = time.Time{}
				item.Schedule = schedule
			},
		},
		{"missing history", func(item *contentapi.Item) { item.Reviews = nil }},
		{
			"invalid schedule",
			func(item *contentapi.Item) {
				schedule := contentapi.Schedule(item.Schedule)
				schedule.State = -1
				item.Schedule = schedule
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			store := openTestStore(t)
			folder, err := storeCreateFolder(
				store,
				&folderapi.CreateInput{Title: "Stored validation"},
			)
			if err != nil {
				t.Fatal(err)
			}
			card, err := storeCreateCard(store,
				&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("content")},
			)
			if err != nil {
				t.Fatal(err)
			}
			item := contentapi.Items(card.Items)[0]
			test.mutate(&item)
			data, err := json.Marshal(item)
			if err != nil {
				t.Fatal(err)
			}
			err = store.update(func(tx *bolt.Tx) error {
				return tx.Bucket([]byte(foldersBucket)).Bucket([]byte(folder.ID)).
					Bucket([]byte(itemsBucket)).
					Bucket([]byte(card.ID)).Put([]byte(contentapi.Items(card.Items)[0].ID), data)
			})
			if err != nil {
				t.Fatal(err)
			}
			if cards, readErr := storeListCards(store); readErr == nil || cards != nil {
				t.Fatalf("invalid stored content returned: %+v, %v", cards, readErr)
			}
		})
	}
}

func TestCardReadFailureDoesNotReturnPartialResults(t *testing.T) {
	store := openTestStore(t)
	folder, err := storeCreateFolder(store, &folderapi.CreateInput{Title: "Partial reads"})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		if _, err = storeCreateCard(store,
			&cardapi.CreateInput{FolderID: folder.ID, Items: textItems("valid")},
		); err != nil {
			t.Fatal(err)
		}
	}
	cards, err := storeListCards(store)
	if err != nil || len(cards) != 2 {
		t.Fatalf("cards=%v error=%v", cards, err)
	}
	err = store.update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(foldersBucket)).
			Bucket([]byte(folder.ID)).
			Bucket([]byte(cardsBucket)).
			Put([]byte(cards[1].ID), []byte("invalid"))
	})
	if err != nil {
		t.Fatal(err)
	}
	if cards, readErr := storeListCards(store); readErr == nil || cards != nil {
		t.Fatalf("returned partial cards after corruption: %v, %v", cards, readErr)
	}
}

func TestRecordEncoderRejectsNonFiniteScheduleValues(t *testing.T) {
	item := contentapi.Item{Schedule: reviewapi.Schedule{Stability: math.NaN()}}
	if _, err := encodeRecord(item); err == nil {
		t.Fatal("encoded non-finite schedule")
	}
}

func configurationFixture() configuration.Settings {
	settings, err := configyaml.Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func storeSettingsFixture() *Settings {
	settings := configurationFixture()
	return &Settings{
		Rules:            configuration.Library(settings),
		MediaRoutePrefix: settings.MediaRoutePrefix(),
		LogErrorKey:      settings.LogErrorKey(),
	}
}

func TestConfiguredContentAndMediaRouteSurvivePersistence(t *testing.T) {
	settings := storeSettingsFixture()
	rules := *rulesValue(settings.Rules)
	rules.TextKind = "plain"
	settings.Rules = rules
	settings.MediaRoutePrefix = "/saved/"
	path := filepath.Join(t.TempDir(), "raven.db")
	deps := NewDependencies(cffbatch.New(), scheduling.New(rulesValue(settings.Rules)), settings)
	store, err := openPersistenceStore(path, deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storeClose(store) })
	folder, err := storeCreateFolder(store, &folderapi.CreateInput{Title: "Configured"})
	if err != nil {
		t.Fatal(err)
	}
	image, contents := mediaFixture(t, ".png")
	card, err := storeCreateCard(store,
		&cardapi.CreateInput{FolderID: folder.ID, Items: []contentapi.Draft{
			{Kind: "plain", Text: "Remember"}, {Kind: "image", SourcePath: image},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := storeClose(store); err != nil {
		t.Fatal(err)
	}
	reopened, err := openPersistenceStore(path, deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storeClose(reopened) })
	cards, err := storeListCards(reopened)
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card}) {
		t.Fatal(cards, err)
	}
	route := "/saved/" + folder.ID + "/" + card.ID + "/" + contentapi.Items(card.Items)[1].ID
	response := httptest.NewRecorder()
	storeMediaHandler(reopened).ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), contents) {
		t.Fatalf("configured media route failed: %d %q", response.Code, response.Body.String())
	}
}

func TestLanguagePreferencePersistenceAndFailures(t *testing.T) {
	storage, err := openStore(filepath.Join(t.TempDir(), "language.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = storeClose(storage) }()
	repository := &Store{state: storage}
	if got, err := repository.LoadLanguagePreference(); err != nil || got != "system" {
		t.Fatal(got, err)
	}
	if err := repository.SaveLanguagePreference("ar"); err != nil {
		t.Fatal(err)
	}
	if got, err := repository.LoadLanguagePreference(); err != nil || got != "ar" {
		t.Fatal(got, err)
	}
	// Existing buckets with no language key retain the System default.
	if err := storage.update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(settingsBucket)).Delete([]byte(languagePreferenceKey))
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := repository.LoadLanguagePreference(); err != nil || got != "system" {
		t.Fatal(got, err)
	}
	// A read-only transaction must report a failed settings commit.
	if err := storage.view(func(tx *bolt.Tx) error {
		if writeLanguagePreference(tx, "en") == nil {
			t.Error("read-only write succeeded")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// A bucket occupying the language key must not be overwritten by a scalar value.
	if err := storage.update(func(tx *bolt.Tx) error {
		_, err := tx.Bucket([]byte(settingsBucket)).CreateBucket([]byte(languagePreferenceKey))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveLanguagePreference("en"); err == nil {
		t.Fatal("bucket was overwritten")
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.LoadLanguagePreference(); err == nil {
		t.Fatal("closed read succeeded")
	}
	if err := repository.SaveLanguagePreference("ar"); err == nil {
		t.Fatal("closed write succeeded")
	}
}

func TestCardTransferCopiesPreserveProgressAndOwnMedia(t *testing.T) {
	store, document, _ := archiveFixture(t)
	original := document.Folders[0].Cards[0]
	destination := document.Folders[1].Metadata.ID
	source := transferapi.Reference{FolderID: original.FolderID, CardID: original.ID}
	request := &transferapi.TransferInput{
		Operation:           "copy",
		Sources:             []transferapi.Reference{source, source},
		DestinationFolderID: destination,
	}
	result, err := transferStoreCards(t, store, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 1 {
		t.Fatal("duplicate source copied twice")
	}
	copied := result.Entries[0].Card
	if copied.ID == original.ID || copied.FolderID != destination ||
		copied.CreatedAt != original.CreatedAt ||
		copied.DueAt != original.DueAt ||
		copied.LastReviewed != original.LastReviewed {
		t.Fatal("copy metadata or progress changed", copied)
	}
	for index, item := range contentapi.Items(copied.Items) {
		previous := contentapi.Items(original.Items)[index]
		if item.ID == previous.ID || item.CardID != copied.ID || item.FolderID != destination ||
			item.Text != previous.Text ||
			item.Hint != previous.Hint ||
			item.Position != previous.Position ||
			item.LastNotifiedDue != 0 {
			t.Fatal("copy identity or content", item)
		}
		if !reflect.DeepEqual(item.Schedule, previous.Schedule) ||
			!reflect.DeepEqual(item.Reviews, previous.Reviews) {
			t.Fatal("copy lost review progress")
		}
		if item.RelativePath != "" {
			before, failure := os.ReadFile(storeMediaPath(store.state, previous.RelativePath))
			after, readErr := os.ReadFile(storeMediaPath(store.state, item.RelativePath))
			if failure != nil || readErr != nil || !bytes.Equal(before, after) ||
				item.RelativePath == previous.RelativePath {
				t.Fatal("copy media ownership", failure, readErr)
			}
		}
	}
	again, err := transferStoreCards(t, store, request)
	if err != nil || again.Entries[0].Card.ID == copied.ID {
		t.Fatal("repeated paste", err)
	}
	if err = store.DeleteCard(source.FolderID, source.CardID); err != nil {
		t.Fatal(err)
	}
	for _, item := range contentapi.Items(copied.Items) {
		if item.RelativePath != "" {
			response := httptest.NewRecorder()
			route := fmt.Sprintf("/media/%s/%s/%s", item.FolderID, item.CardID, item.ID)
			store.MediaHandler().
				ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
			if response.Code != http.StatusOK {
				t.Fatal("copy media lost with original", response.Code)
			}
		}
	}
}

func TestCardTransferMovesMixedSourcesAndCollidingCardIDs(t *testing.T) {
	store, document, _ := archiveFixture(t)
	original := document.Folders[0].Cards[0]
	destination := document.Folders[1].Metadata.ID
	existing, err := storeCreateCard(
		store.state,
		&cardapi.CreateInput{FolderID: destination, Items: textItems("Existing")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if original.ID != existing.ID {
		t.Fatal("fixture must exercise folder-local ID collision")
	}
	sources := []transferapi.Reference{
		{FolderID: original.FolderID, CardID: original.ID},
		{FolderID: destination, CardID: existing.ID},
	}
	result, err := transferStoreCards(t, store,
		&transferapi.TransferInput{
			Operation:           "move",
			Sources:             sources,
			DestinationFolderID: destination,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	moved := result.Entries[0].Card
	if moved.ID == existing.ID || result.Entries[1].Card.ID != existing.ID {
		t.Fatal("move collision or same-folder move changed identity")
	}
	for index, item := range contentapi.Items(moved.Items) {
		previous := contentapi.Items(original.Items)[index]
		previous.FolderID, previous.CardID = destination, moved.ID
		if !reflect.DeepEqual(item, previous) {
			t.Fatal("move changed content or review state")
		}
	}
	if err = store.state.view(func(tx *bolt.Tx) error {
		if _, failure := storeLoadCard(
			store.state,
			tx,
			&[two]string{original.FolderID, original.ID},
		); failure == nil {
			return errors.New("source remains after move")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Same-folder copy still duplicates, while move above was a no-op.
	result, err = transferStoreCards(t, store,
		&transferapi.TransferInput{
			Operation:           "copy",
			Sources:             sources[1:],
			DestinationFolderID: destination,
		},
	)
	if err != nil || result.Entries[0].Card.ID == existing.ID {
		t.Fatal("same-folder copy", err)
	}
}

func TestCardTransferRollsBackInvalidSourcesAndDatabaseFailures(t *testing.T) {
	for _, operation := range []string{"copy", "move"} {
		t.Run(operation, func(t *testing.T) {
			store, document, _ := archiveFixture(t)
			original := document.Folders[0].Cards[0]
			source := transferapi.Reference{FolderID: original.FolderID, CardID: original.ID}
			request := &transferapi.TransferInput{
				Operation:           operation,
				Sources:             []transferapi.Reference{source},
				DestinationFolderID: document.Folders[1].Metadata.ID,
			}
			before, _ := store.ListCards()
			mediaBefore, _ := os.ReadDir(store.state.mediaDir)
			request.Sources = append(
				request.Sources,
				transferapi.Reference{FolderID: source.FolderID, CardID: "missing"},
			)
			if _, err := transferStoreCards(t, store, request); err == nil {
				t.Fatal("missing source accepted")
			}
			request.Sources = request.Sources[:1]
			destination := request.DestinationFolderID
			request.DestinationFolderID = "missing"
			if _, err := transferStoreCards(t, store, request); err == nil {
				t.Fatal("missing destination accepted")
			}
			request.DestinationFolderID = destination
			update := store.state.update
			failure := errors.New("injected commit failure")
			store.state.update = func(action func(*bolt.Tx) error) error {
				return update(func(tx *bolt.Tx) error {
					if err := action(tx); err != nil {
						return err
					}
					return failure
				})
			}
			_, err := transferStoreCards(t, store, request)
			store.state.update = update
			if !errors.Is(err, failure) {
				t.Fatal("lost commit failure", err)
			}
			after, _ := store.ListCards()
			mediaAfter, _ := os.ReadDir(store.state.mediaDir)
			if before != after || !reflect.DeepEqual(mediaBefore, mediaAfter) {
				t.Fatal("failed transfer changed cards or leaked media")
			}
		})
	}
}

func TestCardTransferCopyReloadsConcurrentReview(t *testing.T) {
	store, document, _ := archiveFixture(t)
	original := document.Folders[0].Cards[0]
	update := store.state.update
	var reviewed cardapi.Card
	store.state.update = func(action func(*bolt.Tx) error) error {
		store.state.update = update
		var err error
		reviewed, err = storeReviewItem(
			store.state,
			&cardapi.ReviewInput{
				FolderID: original.FolderID,
				CardID:   original.ID,
				ItemID:   contentapi.Items(original.Items)[0].ID,
				Rating:   4,
			},
		)
		if err != nil {
			return err
		}
		return update(action)
	}
	result, err := transferStoreCards(t, store,
		&transferapi.TransferInput{
			Operation: "copy",
			Sources: []transferapi.Reference{
				{FolderID: original.FolderID, CardID: original.ID},
			},
			DestinationFolderID: document.Folders[1].Metadata.ID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	copied := result.Entries[0].Card
	if !reflect.DeepEqual(
		contentapi.Items(copied.Items)[0].Reviews,
		contentapi.Items(reviewed.Items)[0].Reviews,
	) ||
		copied.LastReviewed != reviewed.LastReviewed ||
		copied.DueAt != reviewed.DueAt {
		t.Fatal("concurrent review was lost")
	}
}

func TestCardTransferCleansStagedMediaAfterReadFailure(t *testing.T) {
	store, document, _ := archiveFixture(t)
	original := document.Folders[0].Cards[0]
	items := contentapi.Items(original.Items)
	if err := os.Remove(storeMediaPath(store.state, items[len(items)-1].RelativePath)); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadDir(store.state.mediaDir)
	_, err := transferStoreCards(t, store,
		&transferapi.TransferInput{
			Operation: "copy",
			Sources: []transferapi.Reference{
				{FolderID: original.FolderID, CardID: original.ID},
			},
			DestinationFolderID: document.Folders[1].Metadata.ID,
		},
	)
	if err == nil {
		t.Fatal("missing media accepted")
	}
	after, _ := os.ReadDir(store.state.mediaDir)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("staged files leaked on media failure")
	}
}

func transferStoreCards(
	t *testing.T,
	store *Store,
	input *transferapi.TransferInput,
) (transferapi.TransferResult, error) {
	t.Helper()
	response, err := store.ApplyCardTransfer(string(encodeFixture(t, input)))
	if err != nil {
		return transferapi.TransferResult{}, err
	}
	var result transferapi.TransferResult
	err = json.Unmarshal([]byte(response), &result)
	if err == nil {
		for index := range result.Entries {
			err = cardapi.Normalize(&result.Entries[index].Card)
			if err != nil {
				break
			}
		}
	}
	return result, err
}

func TestCardTransferCommitAndAllocationFailures(t *testing.T) {
	store, document, _ := archiveFixture(t)
	original := document.Folders[0].Cards[0]
	request := &transferapi.TransferInput{
		Operation: "move",
		Sources: []transferapi.Reference{
			{FolderID: original.FolderID, CardID: original.ID},
		},
		DestinationFolderID: document.Folders[1].Metadata.ID,
	}
	if _, err := transferStoreCards(t, store, &transferapi.TransferInput{}); err == nil {
		t.Fatal("invalid operation accepted")
	}
	transfer := newCardTransfer(store.state, request)
	before, _ := store.ListCards()
	err := store.state.view(func(tx *bolt.Tx) error {
		if _, failure := cardTransferWrite(
			transfer,
			tx,
			0,
		); !errors.Is(
			failure,
			bolt.ErrTxNotWritable,
		) {
			t.Error("read-only transfer", failure)
		}
		if failure := writeTransferredCard(
			tx,
			&original,
		); !errors.Is(
			failure,
			errCardIDAlreadyExists,
		) {
			t.Error("collision accepted", failure)
		}
		invalid := original
		invalid.FolderID = "missing"
		if failure := writeTransferredCard(tx, &invalid); failure == nil {
			t.Error("missing write destination accepted")
		}
		request.DestinationFolderID = "missing"
		if failure := cardTransferCommit(transfer, tx); failure == nil {
			t.Error("missing commit destination accepted")
		}
		request.DestinationFolderID = document.Folders[1].Metadata.ID
		request.Sources[0].CardID = "missing"
		if failure := cardTransferCommit(transfer, tx); failure == nil {
			t.Error("missing commit source accepted")
		}
		request.Sources[0].CardID = original.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	entropy := rand.Reader
	rand.Reader = failedEntropy{}
	t.Cleanup(func() { rand.Reader = entropy })
	_, err = transferStoreCards(t, store, request)
	rand.Reader = entropy
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("allocation failure lost", err)
	}
	err = store.state.update(func(tx *bolt.Tx) error {
		folder := tx.Bucket([]byte(foldersBucket)).Bucket([]byte(original.FolderID))
		if failure := folder.Bucket([]byte(itemsBucket)).
			DeleteBucket([]byte(original.ID)); failure != nil {
			return failure
		}
		_, failure := cardTransferPersist(transfer, tx, &original)
		return failure
	})
	if err == nil {
		t.Fatal("move did not roll back failed source removal")
	}
	after, _ := store.ListCards()
	if before != after {
		t.Fatal("failed transaction changed the library")
	}
}
