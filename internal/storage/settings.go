package storage

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

var settingsBucket = []byte("settings")
var autostartInitializedKey = []byte("autostartInitialized")

// ClaimAutostartInitialization records the attempt before touching OS settings.
// A missing bucket is normal for existing databases; no schema migration is needed.
func (s *Store) ClaimAutostartInitialization() (bool, error) {
	claimed := false
	err := s.db.Update(func(tx *bolt.Tx) error {
		settings, err := tx.CreateBucketIfNotExists(settingsBucket)
		if err != nil {
			return err
		}
		if settings.Get(autostartInitializedKey) != nil {
			return nil
		}
		if err := settings.Put(autostartInitializedKey, []byte("1")); err != nil {
			return err
		}
		claimed = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("record login startup initialization: %w", err)
	}
	return claimed, nil
}
