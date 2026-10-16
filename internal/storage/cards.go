package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"
	bolt "go.etcd.io/bbolt"
)

var itemsBucket = []byte("card_items")

// Card is hydrated for the UI; Items are persisted as independent records.
// DueAt and LastReviewed are derived UI summaries; item.Schedule is authoritative.
type Card struct {
	ID           string `json:"id"`
	FolderID     string `json:"folderId"`
	CreatedAt    int64  `json:"createdAt"`
	DueAt        int64  `json:"dueAt"`
	LastReviewed int64  `json:"lastReviewed"`

	Items []ContentItem `json:"items,omitempty"`
}

type ItemReview struct {
	Log       fsrs.ReviewLog `json:"log"`
	Automatic bool           `json:"automatic"`
}

type ContentItem struct {
	Schedule        fsrs.Card    `json:"schedule"`
	Reviews         []ItemReview `json:"reviews"`
	LastNotifiedDue int64        `json:"lastNotifiedDue"`
	ID              string       `json:"id"`
	FolderID        string       `json:"folderId"`
	CardID          string       `json:"cardId"`
	Position        int          `json:"position"`
	Kind            string       `json:"kind"`
	Text            string       `json:"text,omitempty"`
	OriginalName    string       `json:"originalName,omitempty"`
	MIMEType        string       `json:"mimeType,omitempty"`
	ByteSize        int64        `json:"byteSize,omitempty"`
	RelativePath    string       `json:"relativePath,omitempty"`
}

// ItemDraft carries existing item IDs or a source file for a new media item.
// Positions are assigned by the backend in submission order.
type ItemDraft struct {
	ID         string `json:"id,omitempty"`
	Kind       string `json:"kind"`
	Text       string `json:"text,omitempty"`
	SourcePath string `json:"sourcePath,omitempty"`
}

type CreateCardInput struct {
	FolderID string      `json:"folderId"`
	Items    []ItemDraft `json:"items"`
}

type UpdateCardInput struct {
	FolderID string      `json:"folderId"`
	CardID   string      `json:"cardId"`
	Items    []ItemDraft `json:"items"`
}

func newItemID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

func cardBucket(tx *bolt.Tx, folderID string) (*bolt.Bucket, error) {
	root := tx.Bucket(foldersBucket)
	if root == nil {
		return nil, errors.New("missing folders bucket")
	}
	folder := root.Bucket([]byte(folderID))
	if folder == nil || folder.Bucket(cardsBucket) == nil || folder.Bucket(itemsBucket) == nil {
		return nil, errors.New("folder not found or invalid")
	}
	return folder.Bucket(cardsBucket), nil
}

func loadCard(tx *bolt.Tx, folderID, cardID string) (Card, error) {
	bucket, err := cardBucket(tx, folderID)
	if err != nil {
		return Card{}, err
	}
	data := bucket.Get([]byte(cardID))
	if data == nil {
		return Card{}, errors.New("card not found")
	}
	var card Card
	if err := json.Unmarshal(data, &card); err != nil {
		return Card{}, err
	}
	if card.ID != cardID || card.FolderID != folderID || card.CreatedAt <= 0 {
		return Card{}, errors.New("invalid card metadata")
	}
	items := tx.Bucket(foldersBucket).Bucket([]byte(folderID)).Bucket(itemsBucket).Bucket([]byte(cardID))
	if items == nil {
		return Card{}, errors.New("missing card content")
	}
	card.Items = make([]ContentItem, 0)
	err = items.ForEach(func(id, data []byte) error {
		var item ContentItem
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		if item.ID != string(id) || item.CardID != cardID || item.FolderID != folderID {
			return errors.New("invalid content ownership")
		}
		if err := validateItem(item); err != nil {
			return err
		}
		if meta := tx.Bucket(metaBucket); meta != nil && string(meta.Get(schemaKey)) == "4" {
			if item.Schedule.Due.IsZero() || item.Schedule.LastReview.IsZero() || len(item.Reviews) == 0 {
				return errors.New("missing item schedule")
			}
			if _, err := fsrs.NewFSRS(fsrs.DefaultParam()).Repeat(item.Schedule, item.Schedule.LastReview); err != nil {
				return fmt.Errorf("invalid item schedule: %w", err)
			}
		}
		card.Items = append(card.Items, item)
		return nil
	})
	if err != nil {
		return Card{}, err
	}
	sort.Slice(card.Items, func(i, j int) bool {
		return card.Items[i].Position < card.Items[j].Position
	})
	for position, item := range card.Items {
		if item.Position != position {
			return Card{}, errors.New("invalid content order")
		}
	}
	if len(card.Items) == 0 {
		return Card{}, errors.New("add something you want to remember")
	}
	card.DueAt = 0
	card.LastReviewed = 0
	for _, item := range card.Items {
		due := item.Schedule.Due.UnixMilli()
		if card.DueAt == 0 || due < card.DueAt {
			card.DueAt = due
		}
		for _, review := range item.Reviews {
			if !review.Automatic && review.Log.Review.UnixMilli() > card.LastReviewed {
				card.LastReviewed = review.Log.Review.UnixMilli()
			}
		}
	}
	return card, nil
}

func validateItem(item ContentItem) error {
	if item.ID == "" || item.Position < 0 {
		return errors.New("invalid content item")
	}
	if item.Kind == "text" {
		if strings.TrimSpace(item.Text) == "" || item.RelativePath != "" || item.MIMEType != "" || item.OriginalName != "" || item.ByteSize != 0 {
			return errors.New("invalid text item")
		}
		return nil
	}
	if item.Kind != "audio" && item.Kind != "image" && item.Kind != "video" {
		return errors.New("invalid content kind")
	}
	if item.Text != "" || item.OriginalName == "" || item.ByteSize <= 0 || !strings.HasPrefix(item.MIMEType, item.Kind+"/") || !validMediaPath(item.RelativePath, item.ID) {
		return errors.New("invalid media item")
	}
	return nil
}

func putCard(bucket *bolt.Bucket, card Card) error {
	card.Items = nil
	data, err := json.Marshal(card)
	if err != nil {
		return err
	}
	return bucket.Put([]byte(card.ID), data)
}

func putItems(tx *bolt.Tx, card Card) error {
	root := tx.Bucket(foldersBucket).Bucket([]byte(card.FolderID)).Bucket(itemsBucket)
	if root.Bucket([]byte(card.ID)) != nil {
		if err := root.DeleteBucket([]byte(card.ID)); err != nil {
			return err
		}
	}
	bucket, err := root.CreateBucket([]byte(card.ID))
	if err != nil {
		return err
	}
	for _, item := range card.Items {
		data, err := json.Marshal(item)
		if err != nil {
			return err
		}
		if err := bucket.Put([]byte(item.ID), data); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListCards() ([]Card, error) {
	cards := make([]Card, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		root := tx.Bucket(foldersBucket)
		if root == nil {
			return errors.New("missing folders bucket")
		}
		return root.ForEach(func(folderID, value []byte) error {
			bucket, err := cardBucket(tx, string(folderID))
			if err != nil || value != nil {
				return errors.New("invalid folder bucket")
			}
			return bucket.ForEach(func(id, _ []byte) error {
				card, err := loadCard(tx, string(folderID), string(id))
				if err != nil {
					return fmt.Errorf("read card %q: %w", id, err)
				}
				cards = append(cards, card)
				return nil
			})
		})
	})
	if err != nil {
		return nil, fmt.Errorf("list cards: %w", err)
	}
	return cards, nil
}

func (s *Store) CreateCard(input CreateCardInput) (Card, error) {
	return s.saveCard(input.FolderID, "", input.Items)
}

func (s *Store) UpdateCard(input UpdateCardInput) (Card, error) {
	if input.CardID == "" {
		return Card{}, errors.New("card ID is required")
	}
	return s.saveCard(input.FolderID, input.CardID, input.Items)
}

func (s *Store) saveCard(folderID, cardID string, drafts []ItemDraft) (Card, error) {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	var previous Card
	if err := s.db.View(func(tx *bolt.Tx) error {
		if cardID == "" {
			_, err := cardBucket(tx, folderID)
			return err
		}
		var err error
		previous, err = loadCard(tx, folderID, cardID)
		return err
	}); err != nil {
		return Card{}, err
	}
	items, imported, err := s.prepareItems(drafts, previous.Items)
	committed := false
	defer func() {
		if !committed {
			for _, path := range imported {
				_ = os.Remove(path)
			}
		}
	}()
	if err != nil {
		return Card{}, err
	}
	var card Card
	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket, err := cardBucket(tx, folderID)
		if err != nil {
			return err
		}
		if cardID == "" {
			sequence, err := bucket.NextSequence()
			if err != nil {
				return err
			}
			if sequence == 0 {
				return errors.New("card ID sequence exhausted")
			}
			now := time.Now().UnixMilli()
			card = Card{ID: fmt.Sprintf("%020d", sequence), FolderID: folderID, CreatedAt: now, DueAt: now}
			if bucket.Get([]byte(card.ID)) != nil {
				return errors.New("card ID already exists")
			}
		} else {
			// Read metadata again so a review during file copying is preserved.
			card, err = loadCard(tx, folderID, cardID)
			if err != nil {
				return err
			}
		}
		// Merge fresh scheduling metadata inside the write transaction, preserving
		// reviews and notification acknowledgments made during media imports.
		fresh := make(map[string]ContentItem)
		for _, item := range card.Items {
			fresh[item.ID] = item
		}
		card.Items = append([]ContentItem(nil), items...)
		initializedAt := time.Now()
		for i := range card.Items {
			item := &card.Items[i]
			if old, ok := fresh[item.ID]; ok {
				item.Schedule = old.Schedule
				item.Reviews = old.Reviews
				item.LastNotifiedDue = old.LastNotifiedDue
			} else if err := initializeItem(item, initializedAt); err != nil {
				return err
			}
		}
		for i := range card.Items {
			card.Items[i].FolderID = folderID
			card.Items[i].CardID = card.ID
		}
		if err := putItems(tx, card); err != nil {
			return err
		}
		if err := putCard(bucket, card); err != nil {
			return err
		}
		card, err = loadCard(tx, folderID, card.ID)
		return err
	})
	if err != nil {
		return Card{}, fmt.Errorf("save card: %w", err)
	}
	committed = true
	kept := make(map[string]bool)
	for _, item := range card.Items {
		kept[item.ID] = true
	}
	for _, item := range previous.Items {
		if item.Kind != "text" && !kept[item.ID] {
			_ = os.Remove(s.mediaPath(item.RelativePath))
		}
	}
	return card, nil
}

func (s *Store) prepareItems(drafts []ItemDraft, previous []ContentItem) ([]ContentItem, []string, error) {
	items := make([]ContentItem, 0, len(drafts))
	imported := []string{}
	old := make(map[string]ContentItem)
	for _, item := range previous {
		old[item.ID] = item
	}
	seen := make(map[string]bool)
	for _, draft := range drafts {
		var item ContentItem
		if draft.ID != "" {
			var ok bool
			item, ok = old[draft.ID]
			if !ok || seen[draft.ID] || item.Kind != draft.Kind || draft.SourcePath != "" {
				return nil, imported, errors.New("invalid existing item or ownership")
			}
		} else {
			id, err := newItemID()
			if err != nil {
				return nil, imported, err
			}
			item = ContentItem{ID: id, Kind: draft.Kind}
			if draft.Kind != "text" {
				var path string
				item, path, err = s.importMedia(item, draft.SourcePath)
				if err != nil {
					return nil, imported, err
				}
				imported = append(imported, path)
			}
		}
		if draft.Kind == "text" {
			if draft.SourcePath != "" {
				return nil, imported, errors.New("text cannot reference a file")
			}
			item.Text = strings.TrimSpace(draft.Text)
		} else if draft.Text != "" {
			return nil, imported, errors.New("media cannot contain text")
		}
		item.Position = len(items)
		if err := validateItem(item); err != nil {
			return nil, imported, err
		}
		seen[item.ID] = true
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, imported, errors.New("add something you want to remember")
	}
	return items, imported, nil
}

func initializeItem(item *ContentItem, now time.Time) error {
	result, err := fsrs.NewFSRS(fsrs.DefaultParam()).Next(fsrs.NewCard(now), now, fsrs.Again)
	if err != nil {
		return err
	}
	item.Schedule = result.Card
	item.Reviews = []ItemReview{{Log: result.ReviewLog, Automatic: true}}
	item.LastNotifiedDue = 0
	return nil
}

func (s *Store) ReviewItem(folderID, cardID, itemID string, rating int) (Card, error) {
	return s.reviewItemAt(folderID, cardID, itemID, rating, time.Time{})
}

func (s *Store) reviewItemAt(folderID, cardID, itemID string, rating int, now time.Time) (Card, error) {
	if rating < int(fsrs.Again) || rating > int(fsrs.Easy) {
		return Card{}, errors.New("invalid review rating")
	}
	var card Card
	err := s.db.Update(func(tx *bolt.Tx) error {
		var err error
		card, err = loadCard(tx, folderID, cardID)
		if err != nil {
			return err
		}
		found := false
		for i := range card.Items {
			item := &card.Items[i]
			if item.ID != itemID {
				continue
			}
			if now.IsZero() {
				now = time.Now()
			}
			result, err := fsrs.NewFSRS(fsrs.DefaultParam()).Next(item.Schedule, now, fsrs.Rating(rating))
			if err != nil {
				return err
			}
			item.Schedule = result.Card
			item.Reviews = append(item.Reviews, ItemReview{Log: result.ReviewLog})
			found = true
		}
		if !found {
			return errors.New("content item not found")
		}
		if err := putItems(tx, card); err != nil {
			return err
		}
		card, err = loadCard(tx, folderID, cardID)
		return err
	})
	if err != nil {
		return Card{}, fmt.Errorf("review item: %w", err)
	}
	return card, nil
}
