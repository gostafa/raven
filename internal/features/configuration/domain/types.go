// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package domain defines Raven's embedded configuration contract.
package domain

import (
	companion "raven/internal/features/companion/domain"
	library "raven/internal/shared/boltstore/domain"
)

type (
	// Settings contains the shared values supplied to application consumers.
	Settings struct {
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
