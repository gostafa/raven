// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package library defines library contracts and content workflows.
package library

import (
	"net/http"
	"time"
)

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
type DueOccurrence struct {
	FolderID string
	CardID   string
	ItemID   string
	Due      int64
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
type Scheduler interface {
	Initialize(time.Time) (Schedule, ReviewLog, error)
	Next(Schedule, time.Time, int) (Schedule, ReviewLog, error)
	Validate(Schedule) error
}
type Repository interface {
	ListFolders() ([]Folder, error)
	CreateFolder(CreateFolderInput) (Folder, error)
	DeleteFolder(string) error
	ListCards() ([]Card, error)
	CreateCard(CreateCardInput) (Card, error)
	UpdateCard(UpdateCardInput) (Card, error)
	DeleteCard(string, string) error
	ReviewItem(string, string, string, int) (Card, error)
	MarkNotified([]DueOccurrence) error
	ClaimNotificationPrompt() (bool, error)
	ClaimAutostartInitialization() (bool, error)
	Close() error
}

type SelectedMedia struct {
	SourcePath   string `json:"sourcePath"`
	OriginalName string `json:"originalName"`
	PreviewURL   string `json:"previewURL"`
	ByteSize     int64  `json:"byteSize"`
}
type Media interface {
	Select(string) ([]SelectedMedia, error)
	Middleware(http.Handler) http.Handler
}
