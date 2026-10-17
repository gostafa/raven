// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package yaml loads the embedded application defaults through Uber config.
package yaml

type (
	// Settings exposes independently validated immutable configuration values.
	Settings struct{ values values }
	values   struct {
		// Library contains independently validated library rules.
		Library any `yaml:"library"`
		// Layout contains independently validated window geometry.
		Layout any `yaml:"layout"`
		// ApplicationID identifies the application.
		ApplicationID string `yaml:"application_id"`
		// LogErrorKey names structured error fields.
		LogErrorKey string `yaml:"log_error_key"`
		// LogOperationKey names structured operation fields.
		LogOperationKey string `yaml:"log_operation_key"`
		// MediaRoutePrefix names the persisted media route.
		MediaRoutePrefix string `yaml:"media_route_prefix"`
		// PreviewRoutePrefix names the selected media preview route.
		PreviewRoutePrefix string `yaml:"preview_route_prefix"`
		// PreferencesStorageKey identifies frontend preferences.
		PreferencesStorageKey string `yaml:"preferences_storage_key"`
		// SchedulesChanged names schedule change events.
		SchedulesChanged string `yaml:"schedules_changed"`
		// ShowReviews names review session events.
		ShowReviews string `yaml:"show_reviews"`
		// OpenFloatingReview names floating review open events.
		OpenFloatingReview string `yaml:"open_floating_review"`
		// CloseFloatingReview names floating review close events.
		CloseFloatingReview string `yaml:"close_floating_review"`
	}
)
