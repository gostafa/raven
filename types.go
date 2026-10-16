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

type (
	// RavenService owns the windows and folder database for the lifetime of the app.
	RavenService struct {
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

	// SelectedMedia describes a selected file and its temporary preview URL.
	SelectedMedia struct {
		// SourcePath is the selected local filesystem path.
		SourcePath string `json:"source_path"`
		// OriginalName is the original media filename.
		OriginalName string `json:"original_name"`
		// PreviewURL is the temporary URL used to preview a selected file.
		PreviewURL string `json:"preview_url"`
		// ByteSize is the media size in bytes.
		ByteSize int64 `json:"byte_size"`
	}

	lifecycleAdapter struct{ service *RavenService }

	// Folder contains the metadata of a library folder.
	Folder struct {
		// ID identifies this record.
		ID string `json:"id"`
		// Title is the display title.
		Title string `json:"title"`
		// Description is the optional folder description.
		Description string `json:"description"`
		// Tags contains the normalized folder tags.
		Tags []string `json:"tags"`
	}

	// CreateFolderInput contains the editable metadata for a new folder.
	CreateFolderInput struct {
		// Title is the display title.
		Title string `json:"title"`
		// Description is the optional folder description.
		Description string `json:"description"`
		// Tags contains the normalized folder tags.
		Tags []string `json:"tags"`
	}

	// Card contains ordered content and its aggregate review times.
	Card struct {
		// ID identifies this record.
		ID string `json:"id"`
		// FolderID identifies the owning folder.
		FolderID string `json:"folder_id"`
		// Items contains the ordered card content.
		Items []ContentItem `json:"items,omitempty"`
		// CreatedAt is the card creation time in Unix milliseconds.
		CreatedAt int64 `json:"created_at"`
		// DueAt is the earliest item due time in Unix milliseconds.
		DueAt int64 `json:"due_at"`
		// LastReviewed is the most recent manual review time in Unix milliseconds.
		LastReviewed int64 `json:"last_reviewed"`
	}

	// ItemReview records a scheduling decision and whether it was automatic.
	ItemReview struct {
		// Log contains the scheduling result for this review.
		Log ReviewLog `json:"log"`
		// Automatic marks initialization rather than a user rating.
		Automatic bool `json:"automatic"`
	}

	// ContentItem contains one text or media item and its review history.
	ContentItem struct {
		// Text contains plain text for a text item.
		Text string `json:"text,omitempty"`
		// OriginalName is the original media filename.
		OriginalName string `json:"original_name,omitempty"`
		// ID identifies this record.
		ID string `json:"id"`
		// FolderID identifies the owning folder.
		FolderID string `json:"folder_id"`
		// CardID identifies the owning card.
		CardID string `json:"card_id"`
		// Kind identifies text, image, audio, or video content.
		Kind string `json:"kind"`
		// MIMEType is the validated media content type.
		MIMEType string `json:"mime_type,omitempty"`
		// RelativePath is the media path relative to Raven storage.
		RelativePath string `json:"relative_path,omitempty"`
		// Reviews contains the item scheduling history.
		Reviews []ItemReview `json:"reviews"`
		// Schedule contains the current item scheduling state.
		Schedule Schedule `json:"schedule"`
		// LastNotifiedDue is the last acknowledged due time in Unix milliseconds.
		LastNotifiedDue int64 `json:"last_notified_due"`
		// Position is the zero-based position within the card.
		Position int `json:"position"`
		// ByteSize is the media size in bytes.
		ByteSize int64 `json:"byte_size,omitempty"`
	}

	// ItemDraft describes new content or an edit to an existing item.
	ItemDraft struct {
		// ID identifies this record.
		ID string `json:"id,omitempty"`
		// Kind identifies text, image, audio, or video content.
		Kind string `json:"kind"`
		// Text contains plain text for a text item.
		Text string `json:"text,omitempty"`
		// SourcePath is the selected local filesystem path.
		SourcePath string `json:"source_path,omitempty"`
	}

	// CreateCardInput identifies a folder and the content of a new card.
	CreateCardInput struct {
		// FolderID identifies the owning folder.
		FolderID string `json:"folder_id"`
		// Items contains the ordered card content.
		Items []ItemDraft `json:"items"`
	}

	// UpdateCardInput identifies an existing card and its replacement content.
	UpdateCardInput struct {
		// FolderID identifies the owning folder.
		FolderID string `json:"folder_id"`
		// CardID identifies the owning card.
		CardID string `json:"card_id"`
		// Items contains the ordered card content.
		Items []ItemDraft `json:"items"`
	}

	// Schedule contains the current spaced repetition state of a content item.
	Schedule struct {
		// Due is the next scheduled review time.
		Due time.Time `json:"due"`
		// LastReview is the previous scheduled review time.
		LastReview time.Time `json:"last_review"`
		// Stability is the estimated memory stability in days.
		Stability float64 `json:"stability"`
		// Difficulty is the estimated difficulty of the content.
		Difficulty float64 `json:"difficulty"`
		// ScheduledDays is the interval assigned by the scheduler in days.
		ScheduledDays uint64 `json:"scheduled_days"`
		// Reps counts completed scheduling steps.
		Reps uint64 `json:"reps"`
		// Lapses counts forgotten reviews.
		Lapses uint64 `json:"lapses"`
		// RemainingSteps counts the remaining learning steps.
		RemainingSteps int `json:"remaining_steps"`
		// State identifies the scheduler learning phase.
		State int8 `json:"state"`
	}

	// ReviewLog records the spaced repetition state at a review.
	ReviewLog struct {
		// Due is the next scheduled review time.
		Due time.Time `json:"due"`
		// Review is the time the review was recorded.
		Review time.Time `json:"review"`
		// ScheduledDays is the interval assigned by the scheduler in days.
		ScheduledDays uint64 `json:"scheduled_days"`
		// Stability is the estimated memory stability in days.
		Stability float64 `json:"stability"`
		// Difficulty is the estimated difficulty of the content.
		Difficulty float64 `json:"difficulty"`
		// RemainingSteps counts the remaining learning steps.
		RemainingSteps int `json:"remaining_steps"`
		// Rating is the user rating supplied to the scheduler.
		Rating int8 `json:"rating"`
		// State identifies the scheduler learning phase.
		State int8 `json:"state"`
	}

	// ServiceError exposes a message without retaining an implementation error chain.
	ServiceError struct{ Message string }
)
