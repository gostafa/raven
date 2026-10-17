// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package domain defines Raven's embedded configuration contract.
package domain

import (
	companion "raven/internal/features/companion/domain"
	library "raven/internal/shared/boltstore/domain"
)

type (
	// Settings supplies the immutable configuration shared by application consumers.
	Settings interface {
		identity
		logging
		media
		events
		reviews
	}
	// identity supplies application and preferences identifiers.
	identity interface {
		ApplicationID() string
		PreferencesStorageKey() string
	}
	// logging supplies structured log field names.
	logging interface {
		LogErrorKey() string
		LogOperationKey() string
	}
	// media supplies the desktop media route prefixes.
	media interface {
		MediaRoutePrefix() string
		PreviewRoutePrefix() string
	}
	// events supplies the frontend event names.
	events interface {
		SchedulesChanged() string
		ShowReviews() string
		OpenFloatingReview() string
		CloseFloatingReview() string
	}
	// reviews supplies review rules and floating window geometry by value.
	reviews interface {
		Library() library.Rules
		Layout() companion.Layout
	}
)
