// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package boltstore defines atomic library persistence and legacy schemas.
package boltstore

import (
	"sync"

	bolt "go.etcd.io/bbolt"

	"raven/internal/library"
)

type store struct {
	scheduler library.Scheduler
	db        *bolt.DB
	mediaDir  string
	mediaMu   sync.Mutex
}

type legacyItem struct {
	Side string `json:"side"`
	library.ContentItem
}
