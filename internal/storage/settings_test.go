package storage

import (
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestAutostartMarkerPersistsWithoutChangingContentOrSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	folder, err := s.CreateFolder(
		CreateFolderInput{Title: "Existing folder", Tags: []string{"retained"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err := s.CreateCard(
		CreateCardInput{FolderID: folder.ID, Items: textItems("Existing memory")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if first, err := s.ClaimAutostartInitialization(); err != nil || !first {
		t.Fatalf("first claim=%v error=%v", first, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if first, err := s.ClaimAutostartInitialization(); err != nil || first {
		t.Fatalf("reopened claim=%v error=%v", first, err)
	}
	folders, err := s.ListFolders()
	if err != nil || !reflect.DeepEqual(folders, []Folder{folder}) {
		t.Fatalf("folders changed: %+v, %v", folders, err)
	}
	cards, err := s.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []Card{card}) {
		t.Fatalf("cards changed: %+v, %v", cards, err)
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		if string(tx.Bucket(metaBucket).Get(schemaKey)) != "4" {
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
			first, err := s.ClaimAutostartInitialization()
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
	_ = s.Close()
	if first, err := s.ClaimAutostartInitialization(); err == nil || first {
		t.Fatalf("closed database claim=%v error=%v", first, err)
	}
}
