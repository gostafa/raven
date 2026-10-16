package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

var metaBucket = []byte("meta")
var schemaKey = []byte("schemaVersion")

func (s *Store) initialize(path string) error {
	version := "1"
	existing := false
	err := s.db.View(func(tx *bolt.Tx) error {
		if meta := tx.Bucket(metaBucket); meta != nil && len(meta.Get(schemaKey)) > 0 {
			version = string(meta.Get(schemaKey))
		}
		existing = tx.Bucket(foldersBucket) != nil
		if version != "1" && version != "2" && version != "3" && version != "4" {
			return fmt.Errorf("unsupported database schema %q", version)
		}
		if version != "1" && !existing {
			return errors.New("missing folders bucket")
		}
		return nil
	})
	if err != nil || version == "4" {
		return err
	}
	if existing {
		// Never overwrite a previous backup, including one from a failed attempt.
		backup, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".v"+version+"-*.bak")
		if err != nil {
			return fmt.Errorf("create migration backup: %w", err)
		}
		name := backup.Name()
		err = s.db.View(func(tx *bolt.Tx) error { _, err := tx.WriteTo(backup); return err })
		if err == nil {
			err = backup.Sync()
		}
		closeErr := backup.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(name)
			return fmt.Errorf("write migration backup: %w", err)
		}
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return fmt.Errorf("sync migration backup: %w", err)
		}
	}
	now := time.Now()
	return s.db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		root, err := tx.CreateBucketIfNotExists(foldersBucket)
		if err != nil {
			return err
		}
		err = root.ForEach(func(folderID, value []byte) error {
			folder := root.Bucket(folderID)
			if value != nil || folder == nil || folder.Bucket(cardsBucket) == nil {
				return errors.New("invalid legacy folder")
			}
			if version == "2" && folder.Bucket(itemsBucket) == nil {
				return errors.New("missing legacy content bucket")
			}
			if _, err := folder.CreateBucketIfNotExists(itemsBucket); err != nil {
				return err
			}
			bucket := folder.Bucket(cardsBucket)
			// Collect keys first; do not mutate a bucket while iterating its cursor.
			ids := []string{}
			if err := bucket.ForEach(func(id, _ []byte) error { ids = append(ids, string(id)); return nil }); err != nil {
				return err
			}
			for _, id := range ids {
				data := bucket.Get([]byte(id))
				var card Card
				if err := json.Unmarshal(data, &card); err != nil {
					return err
				}
				if card.ID != id || card.FolderID != string(folderID) {
					return fmt.Errorf("invalid legacy card ownership %q", id)
				}
				card.Items = []ContentItem{}
				if version == "1" {
					var old struct {
						Prompt string `json:"prompt"`
						Answer string `json:"answer"`
					}
					if err := json.Unmarshal(data, &old); err != nil {
						return err
					}
					if strings.TrimSpace(old.Prompt) == "" || strings.TrimSpace(old.Answer) == "" {
						return fmt.Errorf("invalid legacy card %q", id)
					}
					for position, text := range []string{old.Prompt, old.Answer} {
						itemID, err := newItemID()
						if err != nil {
							return err
						}
						card.Items = append(card.Items, ContentItem{ID: itemID, FolderID: card.FolderID, CardID: id, Position: position, Kind: "text", Text: text})
					}
				} else if version == "3" {
					legacy, err := loadCard(tx, card.FolderID, id)
					if err != nil {
						return err
					}
					card.Items = legacy.Items
				} else {
					card.Items, err = mergeLegacyItems(folder.Bucket(itemsBucket).Bucket([]byte(id)), card)
					if err != nil {
						return err
					}
				}
				for i := range card.Items {
					if err := initializeItem(&card.Items[i], now); err != nil {
						return err
					}
				}
				if err := putItems(tx, card); err != nil {
					return err
				}
				if err := putCard(bucket, card); err != nil {
					return err
				}
				if _, err := loadCard(tx, card.FolderID, id); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		return meta.Put(schemaKey, []byte("4"))
	})
}

// Merge prompt items followed by answer items, retaining every item ID and file.
func mergeLegacyItems(bucket *bolt.Bucket, card Card) ([]ContentItem, error) {
	if bucket == nil {
		return nil, errors.New("missing legacy card content")
	}
	type legacyItem struct {
		ContentItem
		Side string `json:"side"`
	}
	old := []legacyItem{}
	err := bucket.ForEach(func(id, data []byte) error {
		var item legacyItem
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		if item.ID != string(id) || item.CardID != card.ID || item.FolderID != card.FolderID || (item.Side != "prompt" && item.Side != "answer") {
			return errors.New("invalid legacy content ownership or side")
		}
		if err := validateItem(item.ContentItem); err != nil {
			return err
		}
		old = append(old, item)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(old, func(i, j int) bool {
		if old[i].Side != old[j].Side {
			return old[i].Side == "prompt"
		}
		return old[i].Position < old[j].Position
	})
	counts := map[string]int{"prompt": 0, "answer": 0}
	items := make([]ContentItem, 0, len(old))
	for position, item := range old {
		if item.Position != counts[item.Side] {
			return nil, errors.New("invalid legacy content order")
		}
		counts[item.Side]++
		item.Position = position
		items = append(items, item.ContentItem)
	}
	if counts["prompt"] == 0 || counts["answer"] == 0 {
		return nil, errors.New("missing legacy card content")
	}
	return items, nil
}
