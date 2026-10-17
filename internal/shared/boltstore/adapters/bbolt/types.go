// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package bbolt defines atomic library persistence and legacy schemas.
package bbolt

import (
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	bolt "go.etcd.io/bbolt"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	library "raven/internal/shared/boltstore/domain"
	"raven/internal/shared/boltstore/ports"
)

type (
	// Dependencies supplies stable runtime dependencies and shared persistence settings.
	Dependencies struct {
		Batch     ports.BatchExecutor
		Scheduler ports.Scheduler
		Logger    *slog.Logger
		Settings  Settings
	}
	legacyContentLoader func(*Settings, *bolt.Bucket, *cardapi.Card) ([]contentapi.Item, error)

	// Settings supplies content rules, media routes, and log field names.
	Settings struct {
		MediaRoutePrefix string
		LogErrorKey      string
		Rules            library.Rules
	}

	// Store owns atomic database and media persistence.
	Store struct {
		batch     ports.BatchExecutor
		settings  *Settings
		scheduler ports.Scheduler
		logger    *slog.Logger
		db        *bolt.DB
		path      string
		mediaDir  string
		mediaMu   sync.Mutex
		ready     atomic.Bool
	}

	legacyItem struct {
		*contentapi.Item

		Side string `json:"side"`
	}
	migrationFolder struct {
		parent *bolt.Bucket
		items  *bolt.Bucket
	}
	schemaState struct {
		version  string
		existing bool
	}
	schemaMigration struct {
		now     time.Time
		storage *Store
		state   schemaState
	}
	cardContentReader struct {
		settings  *Settings
		card      *cardapi.Card
		scheduler ports.Scheduler
		schema    string
	}
	// cardWrite owns the staged files and database transaction for one card edit.
	cardWrite struct {
		storage   *Store
		folderID  string
		cardID    string
		items     []contentapi.Item
		imported  []string
		previous  cardapi.Card
		card      cardapi.Card
		committed bool
	}
	cardRating struct {
		now      time.Time
		storage  *Store
		folderID string
		cardID   string
		itemID   string
		card     cardapi.Card
		rating   int
	}
	cardListing struct {
		storage  *Store
		root     *bolt.Bucket
		items    *bolt.Bucket
		schema   string
		folderID string
		cards    []cardapi.Card
	}
	notificationCard struct {
		items  map[string]*contentapi.Item
		bucket *bolt.Bucket
	}
	folderListing struct {
		root    *bolt.Bucket
		folders []folderapi.Folder
	}
	legacyText struct {
		Prompt string `json:"prompt"`
		Answer string `json:"answer"`
	}
	ownedMedia struct {
		file *os.File
		item *contentapi.Item
	}
)
