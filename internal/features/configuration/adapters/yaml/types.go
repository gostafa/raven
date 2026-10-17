// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package yaml loads the embedded application defaults through Uber config.
package yaml

import (
	companion "raven/internal/features/companion/domain"
	library "raven/internal/shared/boltstore/domain"
)

type (
	// Loader supplies immutable embedded YAML sources.
	Loader   struct{}
	settings struct{ values values }
	values   struct {
		ApplicationID         string           `yaml:"application_id"`
		LogErrorKey           string           `yaml:"log_error_key"`
		LogOperationKey       string           `yaml:"log_operation_key"`
		MediaRoutePrefix      string           `yaml:"media_route_prefix"`
		PreviewRoutePrefix    string           `yaml:"preview_route_prefix"`
		PreferencesStorageKey string           `yaml:"preferences_storage_key"`
		SchedulesChanged      string           `yaml:"schedules_changed"`
		ShowReviews           string           `yaml:"show_reviews"`
		OpenFloatingReview    string           `yaml:"open_floating_review"`
		CloseFloatingReview   string           `yaml:"close_floating_review"`
		Library               library.Rules    `yaml:"library"`
		Layout                companion.Layout `yaml:"layout"`
	}
)
