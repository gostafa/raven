// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package zipfile streams portable archives using Go's ZIP and filesystem support.
package zipfile

import (
	"archive/zip"

	"raven/internal/features/folderarchive/domain"
	"raven/internal/features/folderarchive/ports"
)

type (
	archiveWrite = struct {
		document *domain.Document
		open     ports.OpenMedia
		filename string
	}
	mediaEntry = struct {
		name string
		size int64
	}

	// Archives implements portable archive storage without persistent resources.
	Archives      struct{}
	archiveReader = struct {
		files map[string]*zip.File
	}
)
