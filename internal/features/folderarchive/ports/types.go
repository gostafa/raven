// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines the capabilities used by folder archive workflows.
package ports

import (
	"io"

	"raven/internal/features/folderarchive/domain"
)

type (
	// Transfer exposes the archive workflows to the desktop facade.
	Transfer interface {
		ExportFolders(input string) (domain.Result, error)
		ImportFolders(mode string) (domain.Result, error)
	}
	// OpenMedia streams one archive-relative file while the snapshot remains open.
	OpenMedia = func(name string) (io.ReadCloser, error)
	// Consume processes an owned manifest and its media within their resource scope.
	Consume = func(manifest string, open OpenMedia) error
	// Library provides consistent exports and transactional copy imports.
	Library interface {
		SnapshotFolders(ids []string, consume Consume) error
		ImportFolderCopies(manifest string, mode string, open OpenMedia) ([]string, error)
	}
	// Archives reads and atomically writes the portable file format.
	Archives interface {
		Write(filename string, manifest string, open OpenMedia) error
		Read(filename string, consume Consume) error
	}
	// Dialogs selects authorized import sources and export destinations.
	Dialogs interface {
		SaveArchive() (string, error)
		OpenArchive() (string, error)
	}
)
