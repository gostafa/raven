// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package configuration defines the public frontend configuration snapshot.
package configuration

type (
	// Settings contains only the shared values used by frontend consumers.
	Settings struct {
		PreferencesStorageKey string `json:"preferences_storage_key"`
		MediaRoutePrefix      string `json:"media_route_prefix"`
		TextKind              string `json:"text_kind"`
		SchedulesChanged      string `json:"schedules_changed"`
		ShowReviews           string `json:"show_reviews"`
		OpenFloatingReview    string `json:"open_floating_review"`
		CloseFloatingReview   string `json:"close_floating_review"`
	}
)
