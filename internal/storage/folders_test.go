package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "raven.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestEmptyDatabaseAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven", "raven.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	folders, err := store.ListFolders()
	if err != nil || folders == nil || len(folders) != 0 {
		t.Fatalf("empty database: folders=%v, error=%v", folders, err)
	}
	data, err := json.Marshal(folders)
	if err != nil || string(data) != "[]" {
		t.Fatalf("expected JSON [], got %s, error=%v", data, err)
	}
	if runtime.GOOS != "windows" {
		for name, want := range map[string]os.FileMode{filepath.Dir(path): 0700, path: 0600} {
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

func TestFoldersPersistInCreationOrderWithCardsBuckets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	want := make([]Folder, 0)
	for i := range 12 {
		folder, err := store.CreateFolder(CreateFolderInput{
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
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.ListFolders()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("reopened folders: got=%+v, want=%+v, error=%v", got, want, err)
	}
	err = store.db.View(func(tx *bolt.Tx) error {
		root := tx.Bucket(foldersBucket)
		for _, folder := range want {
			bucket := root.Bucket([]byte(folder.ID))
			if bucket == nil || bucket.Get(metadataKey) == nil || bucket.Bucket(cardsBucket) == nil {
				return fmt.Errorf("folder %s missing metadata or cards bucket", folder.ID)
			}
			if key, _ := bucket.Bucket(cardsBucket).Cursor().First(); key != nil {
				return fmt.Errorf("folder %s cards bucket is not empty", folder.ID)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := store.CreateFolder(CreateFolderInput{Title: "After restart"})
	if err != nil || next.ID != fmt.Sprintf("%020d", len(want)+1) || next.Tags == nil {
		t.Fatalf("sequence/tags after reopen: %+v, error=%v", next, err)
	}
}

func TestValidationDoesNotWrite(t *testing.T) {
	store := openTestStore(t)
	for _, title := range []string{"", " \n\t ", "\u3000"} {
		if _, err := store.CreateFolder(CreateFolderInput{Title: title}); err == nil {
			t.Errorf("accepted empty title %q", title)
		}
	}
	folders, err := store.ListFolders()
	if err != nil || len(folders) != 0 {
		t.Fatalf("validation wrote folders: %v, error=%v", folders, err)
	}
	folder, err := store.CreateFolder(CreateFolderInput{Title: "Valid"})
	if err != nil || folder.ID != "00000000000000000001" || folder.Tags == nil {
		t.Fatalf("invalid sequence or null tags: %+v, error=%v", folder, err)
	}
}

func TestConcurrentFolderCreation(t *testing.T) {
	store := openTestStore(t)
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			if _, err := store.CreateFolder(CreateFolderInput{Title: fmt.Sprintf("Folder %d", i)}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	folders, err := store.ListFolders()
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
	original, err := store.CreateFolder(CreateFolderInput{Title: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	// Force a collision after NextSequence so the failed transaction must roll it back.
	if err := store.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(foldersBucket).SetSequence(0)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateFolder(CreateFolderInput{Title: "Must not overwrite"}); !errors.Is(err, bolt.ErrBucketExists) {
		t.Fatalf("expected bucket collision, got %v", err)
	}
	if err := store.db.View(func(tx *bolt.Tx) error {
		if sequence := tx.Bucket(foldersBucket).Sequence(); sequence != 0 {
			return fmt.Errorf("failed transaction advanced sequence to %d", sequence)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	folders, err := store.ListFolders()
	if err != nil || !reflect.DeepEqual(folders, []Folder{original}) {
		t.Fatalf("failed transaction changed folders: %+v, error=%v", folders, err)
	}
}

func TestDatabaseLockTimesOut(t *testing.T) {
	store := openTestStore(t)
	start := time.Now()
	other, err := Open(store.db.Path())
	if other != nil {
		_ = other.Close()
	}
	if !errors.Is(err, bolt.ErrTimeout) || !strings.Contains(err.Error(), store.db.Path()) {
		t.Fatalf("expected lock timeout with path, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond || elapsed > 5*time.Second {
		t.Errorf("unexpected lock timeout duration: %v", elapsed)
	}
}

func TestUnreadableDatabaseIsNotReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	original := bytes.Repeat([]byte("not a database"), 1024)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if store != nil {
		_ = store.Close()
	}
	if err == nil || !strings.Contains(err.Error(), path) {
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
		if _, err := store.CreateFolder(CreateFolderInput{Title: "Ideas"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(foldersBucket).Bucket([]byte("00000000000000000002")).Put(metadataKey, []byte("{broken"))
	}); err != nil {
		t.Fatal(err)
	}
	if folders, err := store.ListFolders(); err == nil || folders != nil {
		t.Fatalf("expected error without partial results: %+v, error=%v", folders, err)
	}
}

func TestClosedStoreReturnsErrors(t *testing.T) {
	store := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListFolders(); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("expected closed database error, got %v", err)
	}
	if _, err := store.CreateFolder(CreateFolderInput{Title: "Ideas"}); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("expected closed database error, got %v", err)
	}
}
