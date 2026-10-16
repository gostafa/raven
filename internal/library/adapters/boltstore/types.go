// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package boltstore defines atomic library persistence and legacy schemas.
package boltstore

import (
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	"raven/internal/library"
)

type (
	Store struct {
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
	legacyText struct {
		Prompt string `json:"prompt"`
		Answer string `json:"answer"`
	}
)
