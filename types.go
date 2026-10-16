// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package main provides Raven desktop startup and its public Wails facade.
package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"raven/internal/companion"
	"raven/internal/library"
	"raven/internal/reminders"
)

// RavenService owns the windows and folder database for the lifetime of the app.
type RavenService struct {
	app          *application.App
	mainWindow   *application.WebviewWindow
	widget       *application.WebviewWindow
	reviewWindow *application.WebviewWindow
	store        library.Repository
	media        library.Media
	windows      companion.Windows
	mediaHandler http.Handler
	notifier     *notifications.NotificationService
	reminders    *reminders.Coordinator
	reminderMu   sync.Mutex
}

type SelectedMedia struct {
	SourcePath   string `json:"sourcePath"`
	OriginalName string `json:"originalName"`
	PreviewURL   string `json:"previewURL"`
	ByteSize     int64  `json:"byteSize"`
}

type lifecycleAdapter struct{ service *RavenService }

type Folder struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type CreateFolderInput struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type Card struct {
	ID           string        `json:"id"`
	FolderID     string        `json:"folderId"`
	Items        []ContentItem `json:"items,omitempty"`
	CreatedAt    int64         `json:"createdAt"`
	DueAt        int64         `json:"dueAt"`
	LastReviewed int64         `json:"lastReviewed"`
}

type ItemReview struct {
	Log       ReviewLog `json:"log"`
	Automatic bool      `json:"automatic"`
}

type ContentItem struct {
	Text            string       `json:"text,omitempty"`
	OriginalName    string       `json:"originalName,omitempty"`
	ID              string       `json:"id"`
	FolderID        string       `json:"folderId"`
	CardID          string       `json:"cardId"`
	Kind            string       `json:"kind"`
	MIMEType        string       `json:"mimeType,omitempty"`
	RelativePath    string       `json:"relativePath,omitempty"`
	Reviews         []ItemReview `json:"reviews"`
	Schedule        Schedule     `json:"schedule"`
	LastNotifiedDue int64        `json:"lastNotifiedDue"`
	Position        int          `json:"position"`
	ByteSize        int64        `json:"byteSize,omitempty"`
}

type ItemDraft struct {
	ID         string `json:"id,omitempty"`
	Kind       string `json:"kind"`
	Text       string `json:"text,omitempty"`
	SourcePath string `json:"sourcePath,omitempty"`
}

type CreateCardInput struct {
	FolderID string      `json:"folderId"`
	Items    []ItemDraft `json:"items"`
}

type UpdateCardInput struct {
	FolderID string      `json:"folderId"`
	CardID   string      `json:"cardId"`
	Items    []ItemDraft `json:"items"`
}

type Schedule struct {
	Due            time.Time `json:"Due"`
	LastReview     time.Time `json:"LastReview"`
	Stability      float64   `json:"Stability"`
	Difficulty     float64   `json:"Difficulty"`
	ScheduledDays  uint64    `json:"ScheduledDays"`
	Reps           uint64    `json:"Reps"`
	Lapses         uint64    `json:"Lapses"`
	RemainingSteps int       `json:"RemainingSteps"`
	State          int8      `json:"State"`
}

type ReviewLog struct {
	Due            time.Time `json:"Due"`
	Review         time.Time `json:"Review"`
	ScheduledDays  uint64    `json:"ScheduledDays"`
	Stability      float64   `json:"Stability"`
	Difficulty     float64   `json:"Difficulty"`
	RemainingSteps int       `json:"RemainingSteps"`
	Rating         int8      `json:"Rating"`
	State          int8      `json:"State"`
}

// ServiceError exposes a message without retaining an implementation error chain.
type ServiceError struct{ Message string }
