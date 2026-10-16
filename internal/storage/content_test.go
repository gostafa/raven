package storage

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

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
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path, data
}

func itemDrafts(card Card) []ItemDraft {
	drafts := make([]ItemDraft, 0, len(card.Items))
	for _, item := range card.Items {
		drafts = append(drafts, ItemDraft{ID: item.ID, Kind: item.Kind, Text: item.Text})
	}
	return drafts
}

func TestMixedContentPersistenceEditingAndMediaCopies(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(CreateFolderInput{Title: "Mixed"})
	drafts := []ItemDraft{}
	for batch := range 2 {
		for i := range 2 {
			drafts = append(drafts, ItemDraft{Kind: "text", Text: fmt.Sprintf(" text %d %d ", batch, i)})
			for _, media := range []struct{ kind, ext string }{{"image", ".png"}, {"audio", ".wav"}, {"video", ".mp4"}} {
				path, _ := mediaFixture(t, media.ext)
				drafts = append(drafts, ItemDraft{Kind: media.kind, SourcePath: path})
			}
		}
	}
	card, err := s.CreateCard(CreateCardInput{FolderID: folder.ID, Items: drafts})
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
		content := tx.Bucket(foldersBucket).Bucket([]byte(folder.ID)).Bucket(itemsBucket).Bucket([]byte(card.ID))
		if content.Stats().KeyN != 16 {
			t.Fatal("items are not separate records")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	card, err = s.ReviewItem(folder.ID, card.ID, card.Items[0].ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	due, reviewed := card.DueAt, card.LastReviewed
	edit := itemDrafts(card)
	removed := card.Items[1]
	edit = append(edit[:1], edit[2:]...)
	edit[0], edit[1] = edit[1], edit[0]
	edit[1].Text = "Updated text"
	updated, err := s.UpdateCard(UpdateCardInput{FolderID: folder.ID, CardID: card.ID, Items: edit})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DueAt != due || updated.LastReviewed != reviewed || updated.CreatedAt != card.CreatedAt {
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
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cards, err := reopened.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []Card{updated}) {
		t.Fatalf("reopen mismatch: %v", err)
	}
}

func TestContentValidationAndRollback(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(CreateFolderInput{Title: "First"})
	other, _ := s.CreateFolder(CreateFolderInput{Title: "Second"})
	card, err := s.CreateCard(CreateCardInput{FolderID: folder.ID, Items: textItems("Prompt", "Answer")})
	if err != nil {
		t.Fatal(err)
	}
	otherCard, err := s.CreateCard(CreateCardInput{FolderID: other.ID, Items: textItems("Other", "Answer")})
	if err != nil {
		t.Fatal(err)
	}
	// Card IDs are folder scoped, so both may be equal; content IDs still cannot cross ownership.
	foreign := itemDrafts(otherCard)
	duplicate := itemDrafts(card)
	duplicate = append(duplicate, duplicate[0])
	invalid := [][]ItemDraft{
		{}, {{Kind: "text", Text: " "}},
		{{Kind: "html", Text: "Wrong kind"}},
		foreign, duplicate,
		{{ID: card.Items[0].ID, Kind: "image", SourcePath: "anything"}},
	}
	for _, drafts := range invalid {
		if _, err := s.UpdateCard(UpdateCardInput{FolderID: folder.ID, CardID: card.ID, Items: drafts}); err == nil {
			t.Fatalf("accepted invalid drafts: %+v", drafts)
		}
	}
	image, _ := mediaFixture(t, ".png")
	failed := []ItemDraft{{Kind: "image", SourcePath: image}, {Kind: "audio", SourcePath: "missing.wav"}}
	if _, err := s.UpdateCard(UpdateCardInput{FolderID: folder.ID, CardID: card.ID, Items: failed}); err == nil {
		t.Fatal("accepted missing import")
	}
	entries, _ := os.ReadDir(s.mediaDir)
	if len(entries) != 0 {
		t.Fatal("failed import left media files")
	}
	// Force a failure inside the write transaction after a successful copy.
	if err := s.db.Update(func(tx *bolt.Tx) error { bucket, _ := cardBucket(tx, folder.ID); return bucket.SetSequence(0) }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCard(CreateCardInput{FolderID: folder.ID, Items: []ItemDraft{{Kind: "image", SourcePath: image}, {Kind: "text", Text: "Answer"}}}); err == nil {
		t.Fatal("overwrote a colliding card ID")
	}
	entries, _ = os.ReadDir(s.mediaDir)
	if len(entries) != 0 {
		t.Fatal("transaction rollback left imported file")
	}
	cards, err := s.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []Card{card, otherCard}) {
		t.Fatalf("failed edit changed data: %+v, %v", cards, err)
	}
}

func TestMediaOnlyCardsStreamingAndCleanup(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(CreateFolderInput{Title: "Media"})
	other, _ := s.CreateFolder(CreateFolderInput{Title: "Other"})
	image, imageData := mediaFixture(t, ".png")
	audio, _ := mediaFixture(t, ".wav")
	card, err := s.CreateCard(CreateCardInput{FolderID: folder.ID, Items: []ItemDraft{{Kind: "image", SourcePath: image}, {Kind: "audio", SourcePath: audio}}})
	if err != nil {
		t.Fatal(err)
	}
	copyCard, err := s.CreateCard(CreateCardInput{FolderID: folder.ID, Items: []ItemDraft{{Kind: "image", SourcePath: image}, {Kind: "audio", SourcePath: audio}}})
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
		if err := os.WriteFile(filepath.Join(s.mediaDir, name), []byte("orphan"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path := s.db.Path()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
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
	folder, _ := s.CreateFolder(CreateFolderInput{Title: "Media"})
	path := filepath.Join(t.TempDir(), "fake.png")
	if err := os.WriteFile(path, []byte("<html>not an image</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	image, _ := mediaFixture(t, ".png")
	for _, draft := range []ItemDraft{{Kind: "image", SourcePath: path}, {Kind: "video", SourcePath: image}, {Kind: "image", SourcePath: filepath.Dir(image)}} {
		if _, err := s.CreateCard(CreateCardInput{FolderID: folder.ID, Items: append([]ItemDraft{draft}, ItemDraft{Kind: "text", Text: "answer"})}); err == nil {
			t.Fatalf("accepted invalid media: %+v", draft)
		}
	}
	m4a, _ := mediaFixture(t, ".m4a")
	if _, err := s.CreateCard(CreateCardInput{FolderID: folder.ID, Items: []ItemDraft{{Kind: "audio", SourcePath: m4a}, {Kind: "text", Text: "answer"}}}); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentReviewAndContentEdit(t *testing.T) {
	s := openTestStore(t)
	folder, _ := s.CreateFolder(CreateFolderInput{Title: "Concurrent"})
	card, err := s.CreateCard(CreateCardInput{FolderID: folder.ID, Items: textItems("prompt", "answer")})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := s.ReviewItem(folder.ID, card.ID, card.Items[0].ID, 3); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() {
			if _, err := s.UpdateCard(UpdateCardInput{FolderID: folder.ID, CardID: card.ID, Items: itemDrafts(card)}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	cards, err := s.ListCards()
	if err != nil || cards[0].Items[0].Schedule.Reps != 9 || cards[0].LastReviewed == 0 || cards[0].DueAt <= time.Now().UnixMilli() {
		t.Fatalf("edit lost review: %+v, %v", cards, err)
	}
}

func createLegacyDB(t *testing.T, path string, malformed bool) []byte {
	t.Helper()
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"id":"00000000000000000001","folderId":"00000000000000000001","prompt":"Old prompt","answer":"Old answer","createdAt":1000,"dueAt":2000,"lastReviewed":1500,"stage":3}`)
	if malformed {
		data = []byte(`{"id":"bad","folderId":"missing","prompt":"Old prompt","answer":"Old answer","createdAt":1000,"dueAt":2000}`)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		root, _ := tx.CreateBucket(foldersBucket)
		_ = root.SetSequence(1)
		folder, _ := root.CreateBucket([]byte("00000000000000000001"))
		metadata, _ := json.Marshal(Folder{ID: "00000000000000000001", Title: "Legacy", Tags: []string{}})
		_ = folder.Put(metadataKey, metadata)
		cards, _ := folder.CreateBucket(cardsBucket)
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
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := s.ListCards()
	if err != nil || len(cards) != 1 {
		t.Fatalf("migration: %v", err)
	}
	card := cards[0]
	if card.CreatedAt != 1000 || card.DueAt <= 2000 || card.LastReviewed != 0 || len(card.Items) != 2 || card.Items[0].Text != "Old prompt" || card.Items[1].Text != "Old answer" {
		t.Fatalf("migration lost data: %+v", card)
	}
	backups, _ := filepath.Glob(path + ".v1-*.bak")
	if len(backups) != 1 {
		t.Fatal("missing pre-migration backup")
	}
	backup, err := bolt.Open(backups[0], 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	err = backup.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(foldersBucket).Bucket([]byte(card.FolderID)).Bucket(cardsBucket).Get([]byte(card.ID))
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
	s, err = Open(path)
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
	next, err := s.CreateCard(CreateCardInput{FolderID: card.FolderID, Items: textItems("next", "answer")})
	if err != nil || next.ID != "00000000000000000002" {
		t.Fatal("lost card sequence")
	}
}

func TestFailedMigrationLeavesOriginalRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	original := createLegacyDB(t, path, true)
	if s, err := Open(path); err == nil {
		_ = s.Close()
		t.Fatal("accepted invalid legacy ownership")
	}
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(metaBucket) != nil {
			t.Fatal("failed migration committed schema version")
		}
		folder := tx.Bucket(foldersBucket).Bucket([]byte("00000000000000000001"))
		if folder.Bucket(itemsBucket) != nil {
			t.Fatal("failed migration left content records")
		}
		if !bytes.Equal(folder.Bucket(cardsBucket).Get([]byte("00000000000000000001")), original) {
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
	folder, _ := s.CreateFolder(CreateFolderInput{Title: "Information"})
	card, err := s.CreateCard(CreateCardInput{FolderID: folder.ID, Items: textItems("A fact worth remembering")})
	if err != nil || len(card.Items) != 1 {
		t.Fatalf("single text card: %+v, %v", card, err)
	}
	image, _ := mediaFixture(t, ".png")
	updated, err := s.UpdateCard(UpdateCardInput{FolderID: folder.ID, CardID: card.ID, Items: []ItemDraft{{Kind: "image", SourcePath: image}}})
	if err != nil || len(updated.Items) != 1 || updated.Items[0].Kind != "image" || updated.Items[0].Position != 0 {
		t.Fatalf("single media card: %+v, %v", updated, err)
	}
	if _, err := s.UpdateCard(UpdateCardInput{FolderID: folder.ID, CardID: card.ID, Items: nil}); err == nil {
		t.Fatal("accepted an empty information card")
	}
	cards, err := s.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []Card{updated}) {
		t.Fatalf("empty edit changed saved content: %+v, %v", cards, err)
	}
}

func TestSidedContentMigration(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(fmt.Sprintf("malformed=%v", malformed), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "raven.db")
			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			folder, _ := s.CreateFolder(CreateFolderInput{Title: "Existing content"})
			image, _ := mediaFixture(t, ".png")
			card, err := s.CreateCard(CreateCardInput{FolderID: folder.ID, Items: []ItemDraft{{Kind: "text", Text: "Prompt text"}, {Kind: "image", SourcePath: image}, {Kind: "text", Text: "Answer text"}}})
			if err != nil {
				t.Fatal(err)
			}
			card, err = s.ReviewItem(folder.ID, card.ID, card.Items[0].ID, 3)
			if err != nil {
				t.Fatal(err)
			}
			originals := map[string][]byte{}
			err = s.db.Update(func(tx *bolt.Tx) error {
				if err := tx.Bucket(metaBucket).Put(schemaKey, []byte("2")); err != nil {
					return err
				}
				bucket := tx.Bucket(foldersBucket).Bucket([]byte(folder.ID)).Bucket(itemsBucket).Bucket([]byte(card.ID))
				for i, item := range card.Items {
					old := struct {
						ContentItem
						Side string `json:"side"`
					}{ContentItem: item, Side: "prompt"}
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
			s, err = Open(path)
			if malformed {
				if err == nil {
					_ = s.Close()
					t.Fatal("accepted invalid old item order")
				}
				db, err := bolt.Open(path, 0600, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				err = db.View(func(tx *bolt.Tx) error {
					if string(tx.Bucket(metaBucket).Get(schemaKey)) != "2" {
						t.Fatal("failed migration changed version")
					}
					bucket := tx.Bucket(foldersBucket).Bucket([]byte(folder.ID)).Bucket(itemsBucket).Bucket([]byte(card.ID))
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
			want.Items = []ContentItem{card.Items[1], card.Items[0], card.Items[2]}
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
				if string(tx.Bucket(metaBucket).Get(schemaKey)) != "4" {
					t.Fatal("schema not upgraded")
				}
				bucket := tx.Bucket(foldersBucket).Bucket([]byte(folder.ID)).Bucket(itemsBucket).Bucket([]byte(card.ID))
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
			s, err = Open(path)
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
