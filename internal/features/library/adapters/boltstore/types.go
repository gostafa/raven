// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package boltstore defines atomic library persistence and legacy schemas.
package boltstore

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
	library "raven/internal/features/library/domain"
)

type (
	// Store owns atomic database and media persistence.
	Store struct {
		scheduler library.Scheduler
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
		card      *cardapi.Card
		scheduler library.Scheduler
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
		storage     *Store
		transaction *bolt.Tx
		folderID    string
		cards       []cardapi.Card
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
