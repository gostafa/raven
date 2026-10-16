package storage

import (
	"errors"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

func (s *Store) DeleteCard(folderID, cardID string) error {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	err := s.db.Update(func(tx *bolt.Tx) error {
		if _, err := loadCard(tx, folderID, cardID); err != nil {
			return err
		}
		folder := tx.Bucket(foldersBucket).Bucket([]byte(folderID))
		if err := folder.Bucket(itemsBucket).DeleteBucket([]byte(cardID)); err != nil {
			return err
		}
		return folder.Bucket(cardsBucket).Delete([]byte(cardID))
	})
	if err != nil {
		return fmt.Errorf("delete card: %w", err)
	}
	// Any failed file removal is retried by startup cleanup.
	_ = s.cleanupMedia()
	return nil
}

func (s *Store) DeleteFolder(folderID string) error {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	err := s.db.Update(func(tx *bolt.Tx) error {
		root := tx.Bucket(foldersBucket)
		if root == nil || folderID == "" || root.Bucket([]byte(folderID)) == nil {
			return errors.New("folder not found")
		}
		return root.DeleteBucket([]byte(folderID))
	})
	if err != nil {
		return fmt.Errorf("delete folder: %w", err)
	}
	_ = s.cleanupMedia()
	return nil
}
