package storage

import (
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestDeletionPreservesOtherFolders(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "raven.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	first, err := s.CreateFolder(CreateFolderInput{Title: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateFolder(CreateFolderInput{Title: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	create := func(folder string) Card {
		t.Helper()
		card, err := s.CreateCard(CreateCardInput{FolderID: folder, Items: textItems("Remember")})
		if err != nil {
			t.Fatal(err)
		}
		return card
	}
	removed := create(first.ID)
	create(first.ID)
	survivor := create(second.ID)
	if err := s.DeleteCard(first.ID, removed.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(foldersBucket).Bucket([]byte(first.ID)).Bucket(itemsBucket).Bucket([]byte(removed.ID)) != nil {
			t.Error("deleted card still has item records")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cards, err := s.ListCards()
	if err != nil || len(cards) != 2 {
		t.Fatalf("after card deletion: %v, %v", cards, err)
	}
	if err := s.DeleteCard(first.ID, removed.ID); err == nil {
		t.Fatal("expected missing card error")
	}
	if err := s.DeleteFolder(first.ID); err != nil {
		t.Fatal(err)
	}
	cards, err = s.ListCards()
	if err != nil || len(cards) != 1 || cards[0].FolderID != survivor.FolderID || cards[0].ID != survivor.ID {
		t.Fatalf("after folder deletion: %v, %v", cards, err)
	}
	folders, err := s.ListFolders()
	if err != nil || len(folders) != 1 || folders[0].ID != second.ID {
		t.Fatalf("remaining folders: %v, %v", folders, err)
	}
	if err := s.DeleteFolder(first.ID); err == nil {
		t.Fatal("expected missing folder error")
	}
}
