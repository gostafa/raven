// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package application coordinates folder archives through consumer-owned ports.
package application

import (
	"raven/internal/features/folderarchive/domain"
	"raven/internal/features/folderarchive/ports"
)

type (
	archiveOperation = struct {
		open     ports.OpenMedia
		result   *domain.Result
		manifest string
		filename string
		mode     string
	}

	// Service exposes the two folder transfer workflows.
	Service struct {
		operations map[string]func(string) (domain.Result, error)
	}
	workflow = struct {
		library  ports.Library
		archives ports.Archives
		dialogs  ports.Dialogs
	}
)
