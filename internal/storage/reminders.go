package storage

import (
	"errors"
	bolt "go.etcd.io/bbolt"
)

// DueOccurrence identifies a particular schedule, rather than only an item.
type DueOccurrence struct {
	FolderID string
	CardID   string
	ItemID   string
	Due      int64
}

// MarkNotified only acknowledges schedules that have not changed since delivery.
func (s *Store) MarkNotified(occurrences []DueOccurrence) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, occurrence := range occurrences {
			card, err := loadCard(tx, occurrence.FolderID, occurrence.CardID)
			if err != nil {
				return err
			}
			for i := range card.Items {
				item := &card.Items[i]
				if item.ID == occurrence.ItemID && item.Schedule.Due.UnixMilli() == occurrence.Due {
					item.LastNotifiedDue = occurrence.Due
				}
			}
			if err := putItems(tx, card); err != nil {
				return err
			}
		}
		return nil
	})
}

// ClaimNotificationPrompt persists the one-time authorization prompt decision.
func (s *Store) ClaimNotificationPrompt() (bool, error) {
	claimed := false
	err := s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		if meta == nil {
			return errors.New("missing metadata")
		}
		key := []byte("notificationAuthorizationRequested")
		if meta.Get(key) != nil {
			return nil
		}
		claimed = true
		return meta.Put(key, []byte("true"))
	})
	return claimed, err
}
