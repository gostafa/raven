package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var foldersBucket = []byte("folders")
var metadataKey = []byte("metadata")
var cardsBucket = []byte("cards")

type Folder struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type CreateFolderInput struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type Store struct {
	db       *bolt.DB
	mediaDir string
	mediaMu  sync.Mutex
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate Raven configuration directory: %w", err)
	}
	return filepath.Join(dir, "raven", "raven.db"), nil
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create database directory for %q: %w", path, err)
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	s := &Store{db: db, mediaDir: filepath.Join(filepath.Dir(path), "media")}
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

func (s *Store) Close() error {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	return s.db.Close()
}

func (s *Store) ListFolders() ([]Folder, error) {
	folders := make([]Folder, 0)
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
			var folder Folder
			if err := json.Unmarshal(bucket.Get(metadataKey), &folder); err != nil {
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

func (s *Store) CreateFolder(input CreateFolderInput) (Folder, error) {
	folder := Folder{
		Title:       strings.TrimSpace(input.Title),
		Description: strings.TrimSpace(input.Description),
		Tags:        make([]string, 0),
	}
	if folder.Title == "" {
		return Folder{}, errors.New("enter a folder title")
	}
	seen := make(map[string]bool)
	for _, tag := range input.Tags {
		tag = strings.TrimSpace(tag)
		key := strings.ToLower(tag)
		if tag != "" && !seen[key] {
			folder.Tags = append(folder.Tags, tag)
			seen[key] = true
		}
	}
	err := s.db.Update(func(tx *bolt.Tx) error {
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
		return Folder{}, fmt.Errorf("create folder: %w", err)
	}
	return folder, nil
}
