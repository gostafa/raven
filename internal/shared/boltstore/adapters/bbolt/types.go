// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package bbolt defines atomic library persistence and legacy schemas.
package bbolt

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	bolt "go.etcd.io/bbolt"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
)

type (
	// Store exposes transactional persistence through JSON records.
	Store struct{ state *persistence }

	database interface {
		View(callback func(*bolt.Tx) error) error
		Update(callback func(*bolt.Tx) error) error
		Close() error
		Path() string
		Stats() bolt.Stats
	}
	snapshot interface {
		WriteTo(writer io.Writer) (int64, error)
		Size() int64
	}
	durableFile interface {
		Sync() error
		Close() error
	}
	// Dependencies supplies stable runtime dependencies and shared persistence settings.
	Dependencies struct {
		// Batch drains bounded concurrent media jobs.
		Batch func(context.Context, []func(context.Context) error) error
		// Initialize allocates the first scheduling state and review log.
		Initialize func(time.Time) (json.RawMessage, json.RawMessage, error)
		// Next advances a validated schedule after a rating.
		Next func(any, time.Time, int) (json.RawMessage, json.RawMessage, error)
		// Validate verifies a scheduling state.
		Validate func(any) error
		// Logger records cleanup failures.
		Logger *slog.Logger
		// Settings contains immutable validated persistence settings.
		Settings any
	}
	legacyContentLoader func(*Settings, *bolt.Bucket, *cardapi.Card) ([]contentapi.Item, error)

	// Settings supplies content rules, media routes, and log field names.
	Settings struct {
		Rules            any
		MediaRoutePrefix string
		LogErrorKey      string
	}

	// Store owns atomic database and media persistence.
	persistence struct {
		batch              func(context.Context, []func(context.Context) error) error
		settings           any
		initializeSchedule func(time.Time) (json.RawMessage, json.RawMessage, error)
		nextSchedule       func(any, time.Time, int) (json.RawMessage, json.RawMessage, error)
		validateSchedule   func(any) error
		logger             *slog.Logger
		view               func(func(*bolt.Tx) error) error
		update             func(func(*bolt.Tx) error) error
		closeDatabase      func() error
		databasePath       func() string
		databaseStats      func() bolt.Stats

		path     string
		mediaDir string
		mediaMu  sync.Mutex
		ready    atomic.Bool
	}

	legacyItem struct {
		// Item contains the normalized content fields of the flattened legacy record.
		Item any `json:"-"`
		// Side identifies the historical prompt or answer group.
		Side string `json:"side"`
	}
	migrationFolder struct {
		parent *bolt.Bucket
		items  *bolt.Bucket
		id     []byte
	}
	legacyEntry struct {
		id    []byte
		value []byte
	}
	schemaState struct {
		version  string
		existing bool
	}
	schemaMigration struct {
		now         time.Time
		settings    any
		initialize  func(time.Time) (json.RawMessage, json.RawMessage, error)
		writeBackup func(*os.File) error
		loadItems   func(*bolt.Tx, string, string) ([]any, error)
		version     string
		existing    bool
	}
	cardContentReader struct {
		settings any
		card     any
		validate func(any) error
		schema   string
	}
	// cardWrite owns the staged files and database transaction for one card edit.
	cardWrite struct {
		items     any
		previous  any
		card      any
		folderID  string
		cardID    string
		imported  []string
		committed bool
	}
	cardTransaction struct {
		transaction *bolt.Tx
		bucket      *bolt.Bucket
	}
	itemInitialization struct {
		at         time.Time
		initialize func(time.Time) (json.RawMessage, json.RawMessage, error)
	}
	cardRating struct {
		card     any
		item     any
		now      time.Time
		folderID string
		cardID   string
		itemID   string
		rating   int
	}
	cardListing struct {
		cards    any
		root     *bolt.Bucket
		items    *bolt.Bucket
		schema   string
		folderID string
	}
	notificationCard struct {
		items  any
		bucket *bolt.Bucket
	}
	folderListing struct {
		root    *bolt.Bucket
		folders any
	}
	legacyText struct {
		// Prompt contains the original question text.
		Prompt string `json:"prompt"`
		// Answer contains the original answer text.
		Answer string `json:"answer"`
	}
	ownedMedia struct {
		file *os.File
		item any
	}
	mediaImport struct {
		item   any
		file   *os.File
		source string
	}
)
