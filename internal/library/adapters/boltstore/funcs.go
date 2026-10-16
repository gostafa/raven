// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package boltstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"

	"raven/internal/library"
)

// ClaimAutostartInitialization records the attempt before touching OS settings.
// A missing bucket is normal for existing databases; no schema migration is needed.
func (s *store) ClaimAutostartInitialization() (bool, error) {
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

// MarkNotified only acknowledges schedules that have not changed since delivery.
func (s *store) MarkNotified(occurrences []library.DueOccurrence) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, occurrence := range occurrences {
			card, err := s.loadCard(tx, occurrence.FolderID, occurrence.CardID)
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
func (s *store) ClaimNotificationPrompt() (bool, error) {
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

func (s *store) initialize(path string) error {
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

		err = s.db.View(func(tx *bolt.Tx) error {
			_, err := tx.WriteTo(backup)

			return err
		})
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

			err := bucket.ForEach(
				func(id, _ []byte) error {
					ids = append(ids, string(id))

					return nil
				},
			)
			if err != nil {
				return err
			}

			for _, id := range ids {
				data := bucket.Get([]byte(id))

				var card library.Card

				err := json.Unmarshal(data, &card)
				if err != nil {
					return err
				}

				if card.ID != id || card.FolderID != string(folderID) {
					return fmt.Errorf("invalid legacy card ownership %q", id)
				}

				card.Items = []library.ContentItem{}

				switch version {
				case "1":
					var old struct {
						Prompt string `json:"prompt"`
						Answer string `json:"answer"`
					}

					err := json.Unmarshal(data, &old)
					if err != nil {
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

						card.Items = append(
							card.Items,
							library.ContentItem{
								ID:       itemID,
								FolderID: card.FolderID,
								CardID:   id,
								Position: position,
								Kind:     "text",
								Text:     text,
							},
						)
					}
				case "3":
					legacy, err := s.loadCard(tx, card.FolderID, id)
					if err != nil {
						return err
					}

					card.Items = legacy.Items
				default:
					card.Items, err = mergeLegacyItems(
						folder.Bucket(itemsBucket).Bucket([]byte(id)),
						card,
					)
					if err != nil {
						return err
					}
				}

				for i := range card.Items {
					err := library.InitializeItem(&card.Items[i], now, s.scheduler)
					if err != nil {
						return err
					}
				}

				err = putItems(tx, card)
				if err != nil {
					return err
				}

				err = putCard(bucket, card)
				if err != nil {
					return err
				}

				if _, err := s.loadCard(tx, card.FolderID, id); err != nil {
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
func mergeLegacyItems(bucket *bolt.Bucket, card library.Card) ([]library.ContentItem, error) {
	if bucket == nil {
		return nil, errors.New("missing legacy card content")
	}

	old := []legacyItem{}

	err := bucket.ForEach(func(id, data []byte) error {
		var item legacyItem

		err := json.Unmarshal(data, &item)
		if err != nil {
			return err
		}

		if item.ID != string(id) || item.CardID != card.ID || item.FolderID != card.FolderID ||
			(item.Side != "prompt" && item.Side != "answer") {

			return errors.New("invalid legacy content ownership or side")
		}

		err = library.ValidateItem(item.ContentItem)
		if err != nil {
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
	items := make([]library.ContentItem, 0, len(old))

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

func (s *store) mediaPath(relative string) string {
	return filepath.Join(s.mediaDir, filepath.Base(relative))
}

func inspectMedia(file *os.File, kind, source string) (string, error) {
	declared := mediaTypes[strings.ToLower(filepath.Ext(source))]
	if (kind != "audio" && kind != "image" && kind != "video") ||
		!strings.HasPrefix(declared, kind+"/") {

		return "", errors.New("choose a supported file for this media type")
	}

	var header [512]byte

	n, err := file.Read(header[:])

	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	if n == 0 {
		return "", errors.New("empty media file")
	}

	detected, _, _ := strings.Cut(http.DetectContentType(header[:n]), ";")
	// Ogg, WebM, and MP4 containers can carry audio or video. AVIF/FLAC
	// may be reported as octet-stream by Go's sniffer; check their signatures.
	compatible := detected == declared ||
		(declared == "audio/wav" && detected == "audio/wave") ||
		((declared == "audio/ogg" || declared == "video/ogg") && detected == "application/ogg") ||
		(declared == "audio/mp4" && detected == "video/mp4") ||
		(declared == "video/quicktime" && n >= 12 && string(header[4:8]) == "ftyp") ||
		(declared == "image/avif" && n >= 12 && string(header[4:8]) == "ftyp" && (string(header[8:12]) == "avif" || string(header[8:12]) == "avis")) ||
		(declared == "audio/flac" && n >= 4 && string(header[:4]) == "fLaC")
	if !compatible {
		return "", fmt.Errorf("file contents do not match %s", declared)
	}

	_, err = file.Seek(0, io.SeekStart)

	return declared, err
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}

	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()

	return dir.Sync()
}

func (s *store) importMedia(
	item library.ContentItem,
	source string,
) (library.ContentItem, string, error) {
	file, err := os.Open(source)
	if err != nil {
		return item, "", fmt.Errorf("open selected file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return item, "", err
	}

	if !info.Mode().IsRegular() {
		return item, "", errors.New("select a regular media file")
	}

	mimeType, err := inspectMedia(file, item.Kind, source)
	if err != nil {
		return item, "", err
	}

	temp, err := os.CreateTemp(s.mediaDir, ".import-*")
	if err != nil {
		return item, "", err
	}
	defer os.Remove(temp.Name())

	size, copyErr := io.Copy(temp, file)
	if copyErr == nil {
		copyErr = temp.Sync()
	}

	closeErr := temp.Close()

	if copyErr != nil {
		return item, "", copyErr
	}

	if closeErr != nil {
		return item, "", closeErr
	}

	if size != info.Size() {
		return item, "", errors.New("selected file changed during import; try again")
	}

	item.RelativePath = "media/" + item.ID + strings.ToLower(filepath.Ext(source))

	path := s.mediaPath(item.RelativePath)

	if err := os.Rename(temp.Name(), path); err != nil {
		return item, "", err
	}

	if err := syncDirectory(s.mediaDir); err != nil {
		_ = os.Remove(path)

		return item, "", err
	}

	item.OriginalName = filepath.Base(source)
	item.MIMEType = mimeType
	item.ByteSize = size

	return item, path, nil
}

func (s *store) cleanupMedia() error {
	// Validate all referenced records before removing anything.
	cards, err := s.ListCards()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(s.mediaDir, 0o700); err != nil {
		return err
	}

	referenced := make(map[string]bool)

	for _, card := range cards {
		for _, item := range card.Items {
			if item.Kind != "text" {
				referenced[filepath.Base(item.RelativePath)] = true
			}
		}
	}

	entries, err := os.ReadDir(s.mediaDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		managed := strings.HasPrefix(name, ".import-") ||
			library.ValidMediaPath("media/"+name, strings.TrimSuffix(name, filepath.Ext(name)))

		if !entry.IsDir() && managed && !referenced[name] {
			err := os.Remove(filepath.Join(s.mediaDir, name))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// MediaHandler serves only media records owned by the requested card.
// ServeContent implements HEAD and byte ranges without buffering the file.
func (s *store) MediaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)

			return
		}

		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/media/"), "/")
		if !strings.HasPrefix(r.URL.Path, "/media/") || len(parts) != 3 ||
			!itemIDPattern.MatchString(parts[2]) {

			http.NotFound(w, r)

			return
		}

		var item library.ContentItem

		s.mediaMu.Lock()

		err := s.db.View(func(tx *bolt.Tx) error {
			card, err := s.loadCard(tx, parts[0], parts[1])
			if err != nil {
				return err
			}

			for _, candidate := range card.Items {
				if candidate.ID == parts[2] && candidate.Kind != "text" {
					item = candidate

					return nil
				}
			}

			return errors.New("media not found")
		})

		var file *os.File

		if err == nil {
			file, err = os.Open(s.mediaPath(item.RelativePath))
		}

		s.mediaMu.Unlock()

		if err != nil {
			http.NotFound(w, r)

			return
		}

		defer file.Close()

		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)

			return
		}

		w.Header().Set("Content-Type", item.MIMEType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		http.ServeContent(w, r, item.OriginalName, info.ModTime(), file)
	})
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate Raven configuration directory: %w", err)
	}

	return filepath.Join(dir, "raven", "raven.db"), nil
}

func Open(path string, scheduler library.Scheduler) (*store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory for %q: %w", path, err)
	}

	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}

	s := &store{db: db, scheduler: scheduler, mediaDir: filepath.Join(filepath.Dir(path), "media")}
	if err := s.initialize(path); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("initialize database %q: %w", path, err)
	}

	if err := s.cleanupMedia(); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("check media for %q: %w", path, err)
	}

	return s, nil
}

func (s *store) Close() error {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()

	return s.db.Close()
}

func (s *store) ListFolders() ([]library.Folder, error) {
	folders := make([]library.Folder, 0)

	err := s.db.View(func(tx *bolt.Tx) error {
		root := tx.Bucket(foldersBucket)
		if root == nil {
			return errors.New("missing folders bucket")
		}

		return root.ForEach(func(key, value []byte) error {
			bucket := root.Bucket(key)
			if value != nil || bucket == nil || bucket.Bucket(cardsBucket) == nil {
				return fmt.Errorf("invalid folder bucket %q", key)
			}

			var folder library.Folder

			err := json.Unmarshal(bucket.Get(metadataKey), &folder)
			if err != nil {
				return fmt.Errorf("read folder %q: %w", key, err)
			}

			if folder.ID != string(key) || strings.TrimSpace(folder.Title) == "" {
				return fmt.Errorf("invalid metadata for folder %q", key)
			}

			if folder.Tags == nil {
				folder.Tags = []string{}
			}

			folders = append(folders, folder)

			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}

	return folders, nil
}

func (s *store) CreateFolder(input library.CreateFolderInput) (library.Folder, error) {
	folder, err := library.NormalizeFolder(input)
	if err != nil {
		return library.Folder{}, err
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		root := tx.Bucket(foldersBucket)
		if root == nil {
			return errors.New("missing folders bucket")
		}

		sequence, err := root.NextSequence()
		if err != nil {
			return err
		}

		if sequence == 0 {
			return errors.New("folder ID sequence exhausted")
		}

		folder.ID = fmt.Sprintf("%020d", sequence)

		bucket, err := root.CreateBucket([]byte(folder.ID))
		if err != nil {
			return err
		}

		metadata, err := json.Marshal(folder)
		if err != nil {
			return err
		}

		if err := bucket.Put(metadataKey, metadata); err != nil {
			return err
		}

		if _, err = bucket.CreateBucket(cardsBucket); err != nil {
			return err
		}

		_, err = bucket.CreateBucket(itemsBucket)

		return err
	})
	if err != nil {
		return library.Folder{}, fmt.Errorf("create folder: %w", err)
	}

	return folder, nil
}

func (s *store) DeleteCard(folderID, cardID string) error {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()

	err := s.db.Update(func(tx *bolt.Tx) error {
		if _, err := s.loadCard(tx, folderID, cardID); err != nil {
			return err
		}

		folder := tx.Bucket(foldersBucket).Bucket([]byte(folderID))

		err := folder.Bucket(itemsBucket).DeleteBucket([]byte(cardID))
		if err != nil {
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

func (s *store) DeleteFolder(folderID string) error {
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

func (s *store) loadCard(tx *bolt.Tx, folderID, cardID string) (library.Card, error) {
	bucket, err := cardBucket(tx, folderID)
	if err != nil {
		return library.Card{}, err
	}

	data := bucket.Get([]byte(cardID))
	if data == nil {
		return library.Card{}, errors.New("card not found")
	}

	var card library.Card

	if err := json.Unmarshal(data, &card); err != nil {
		return library.Card{}, err
	}

	if card.ID != cardID || card.FolderID != folderID || card.CreatedAt <= 0 {
		return library.Card{}, errors.New("invalid card metadata")
	}

	items := tx.Bucket(foldersBucket).
		Bucket([]byte(folderID)).
		Bucket(itemsBucket).
		Bucket([]byte(cardID))
	if items == nil {
		return library.Card{}, errors.New("missing card content")
	}

	card.Items = make([]library.ContentItem, 0)

	err = items.ForEach(func(id, data []byte) error {
		var item library.ContentItem

		err := json.Unmarshal(data, &item)
		if err != nil {
			return err
		}

		if item.ID != string(id) || item.CardID != cardID || item.FolderID != folderID {
			return errors.New("invalid content ownership")
		}

		err = library.ValidateItem(item)
		if err != nil {
			return err
		}

		if meta := tx.Bucket(metaBucket); meta != nil && string(meta.Get(schemaKey)) == "4" {
			if item.Schedule.Due.IsZero() || item.Schedule.LastReview.IsZero() ||
				len(item.Reviews) == 0 {

				return errors.New("missing item schedule")
			}

			err := s.scheduler.Validate(item.Schedule)
			if err != nil {
				return fmt.Errorf("invalid item schedule: %w", err)
			}
		}

		card.Items = append(card.Items, item)

		return nil
	})
	if err != nil {
		return library.Card{}, err
	}

	return library.Hydrate(card)
}

func putCard(bucket *bolt.Bucket, card library.Card) error {
	card.Items = nil

	data, err := json.Marshal(card)
	if err != nil {
		return err
	}

	return bucket.Put([]byte(card.ID), data)
}

func putItems(tx *bolt.Tx, card library.Card) error {
	root := tx.Bucket(foldersBucket).Bucket([]byte(card.FolderID)).Bucket(itemsBucket)
	if root.Bucket([]byte(card.ID)) != nil {
		err := root.DeleteBucket([]byte(card.ID))
		if err != nil {
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

func (s *store) ListCards() ([]library.Card, error) {
	cards := make([]library.Card, 0)

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
				card, err := s.loadCard(tx, string(folderID), string(id))
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

func (s *store) CreateCard(input library.CreateCardInput) (library.Card, error) {
	return s.saveCard(input.FolderID, "", input.Items)
}

func (s *store) UpdateCard(input library.UpdateCardInput) (library.Card, error) {
	if input.CardID == "" {
		return library.Card{}, errors.New("card ID is required")
	}

	return s.saveCard(input.FolderID, input.CardID, input.Items)
}

func (s *store) saveCard(
	folderID, cardID string,
	drafts []library.ItemDraft,
) (library.Card, error) {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()

	var previous library.Card

	if err := s.db.View(func(tx *bolt.Tx) error {
		if cardID == "" {
			_, err := cardBucket(tx, folderID)

			return err
		}

		var err error

		previous, err = s.loadCard(tx, folderID, cardID)

		return err
	}); err != nil {
		return library.Card{}, err
	}

	items, imported, err := library.PrepareItems(drafts, previous.Items, s.importMedia, newItemID)
	committed := false

	defer func() {
		if !committed {
			for _, path := range imported {
				_ = os.Remove(path)
			}
		}
	}()

	if err != nil {
		return library.Card{}, err
	}

	var card library.Card

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

			card = library.Card{
				ID:        fmt.Sprintf("%020d", sequence),
				FolderID:  folderID,
				CreatedAt: now,
				DueAt:     now,
			}

			if bucket.Get([]byte(card.ID)) != nil {
				return errors.New("card ID already exists")
			}
		} else {
			// Read metadata again so a review during file copying is preserved.
			card, err = s.loadCard(tx, folderID, cardID)
			if err != nil {
				return err
			}
		}

		// Merge fresh scheduling metadata inside the write transaction, preserving
		// reviews and notification acknowledgments made during media imports.
		fresh := make(map[string]library.ContentItem)

		for _, item := range card.Items {
			fresh[item.ID] = item
		}

		card.Items = append([]library.ContentItem(nil), items...)

		initializedAt := time.Now()

		for i := range card.Items {
			item := &card.Items[i]
			if old, ok := fresh[item.ID]; ok {
				item.Schedule = old.Schedule
				item.Reviews = old.Reviews
				item.LastNotifiedDue = old.LastNotifiedDue
			} else {
				err := library.InitializeItem(item, initializedAt, s.scheduler)
				if err != nil {
					return err
				}
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

		card, err = s.loadCard(tx, folderID, card.ID)

		return err
	})
	if err != nil {
		return library.Card{}, fmt.Errorf("save card: %w", err)
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

func (s *store) ReviewItem(folderID, cardID, itemID string, rating int) (library.Card, error) {
	return s.reviewItemAt(folderID, cardID, itemID, rating, time.Time{})
}

func (s *store) reviewItemAt(
	folderID, cardID, itemID string,
	rating int,
	now time.Time,
) (library.Card, error) {
	if rating < 1 || rating > 4 {
		return library.Card{}, errors.New("invalid review rating")
	}

	var card library.Card

	err := s.db.Update(func(tx *bolt.Tx) error {
		var err error

		card, err = s.loadCard(tx, folderID, cardID)
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

			err := library.ApplyReview(item, now, rating, s.scheduler)
			if err != nil {
				return err
			}

			found = true
		}

		if !found {
			return errors.New("content item not found")
		}

		if err := putItems(tx, card); err != nil {
			return err
		}

		card, err = s.loadCard(tx, folderID, cardID)

		return err
	})
	if err != nil {
		return library.Card{}, fmt.Errorf("review item: %w", err)
	}

	return card, nil
}
