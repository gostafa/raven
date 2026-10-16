// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package desktopmedia adapts native file selection and HTTP media previews.
package desktopmedia

import (
	"log/slog"
	"net/http"
	"sync"
)

type (
	// Picker owns file selection and the preview capabilities issued to selected paths.
	Picker struct {
		choose   func(kind, filter string) ([]string, error)
		saved    func() http.Handler
		logger   *slog.Logger
		previews sync.Map
	}
)
