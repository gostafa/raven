// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package boltstore defines atomic library persistence and legacy schemas.
package boltstore

import (
	"log/slog"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	"raven/internal/library"
)

type (
	// Store owns atomic database and media persistence.
	Store struct {
		logger    *slog.Logger
		scheduler library.Scheduler
		db        *bolt.DB
		mediaDir  string
		mediaMu   sync.Mutex
	}

	legacyItem struct {
		Side string `json:"side"`
		library.ContentItem
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
		card      *library.Card
		scheduler library.Scheduler
		schema    string
	}
	// cardWrite owns the staged files and database transaction for one card edit.
	cardWrite struct {
		storage   *Store
		folderID  string
		cardID    string
		items     []library.ContentItem
		imported  []string
		previous  library.Card
		card      library.Card
		committed bool
	}
	legacyText struct {
		Prompt string `json:"prompt"`
		Answer string `json:"answer"`
	}
)
