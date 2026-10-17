// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package configuration defines the public frontend configuration snapshot.
package configuration

type (
	// Settings contains only the shared values used by frontend consumers.
	Settings struct {
		// PreferencesStorageKey identifies the frontend's persisted preferences.
		PreferencesStorageKey string `json:"preferences_storage_key"`
		// MediaRoutePrefix is the route used to serve persisted media.
		MediaRoutePrefix string `json:"media_route_prefix"`
		// TextKind identifies plain text content.
		TextKind string `json:"text_kind"`
		// SchedulesChanged announces a change to the library's review schedules.
		SchedulesChanged string `json:"schedules_changed"`
		// ShowReviews requests the main window's review session.
		ShowReviews string `json:"show_reviews"`
		// OpenFloatingReview requests the floating review session.
		OpenFloatingReview string `json:"open_floating_review"`
		// CloseFloatingReview requests that the floating review session close.
		CloseFloatingReview string `json:"close_floating_review"`
	}
)
