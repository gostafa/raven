// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package domain

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	companion "raven/internal/features/companion/domain"
	library "raven/internal/shared/boltstore/domain"
)

// Validate rejects incomplete settings before any native or persistence resources open.
func Validate(settings Settings) error {
	values := requiredStrings(settings)

	for name := range values {
		if strings.TrimSpace(values[name]) == "" {
			return fmt.Errorf("%w: %s is required", errInvalidConfiguration, name)
		}
	}

	err := validateValues(settings)
	if err != nil {
		return fmt.Errorf(validationErrorFormat, err)
	}

	return nil
}

func requiredStrings(settings Settings) map[string]string {
	return map[string]string{
		"application_id":          settings.ApplicationID(),
		"log_error_key":           settings.LogErrorKey(),
		"log_operation_key":       settings.LogOperationKey(),
		"preferences_storage_key": settings.PreferencesStorageKey(),
		"schedules_changed":       settings.SchedulesChanged(),
		"show_reviews":            settings.ShowReviews(),
		"open_floating_review":    settings.OpenFloatingReview(),
		"close_floating_review":   settings.CloseFloatingReview(),
		"library.text_kind":       Library(settings).TextKind,
		"library.mime_jpeg":       Library(settings).MIMEJPEG,
		"library.mime_ogg_audio":  Library(settings).MIMEOggAudio,
		"library.mime_mp4_video":  Library(settings).MIMEMP4Video,
	}
}

func validateValues(settings Settings) error {
	values := geometry(settings)

	for name := range values {
		if values[name] <= zero {
			return fmt.Errorf("%w: %s must be positive", errInvalidConfiguration, name)
		}
	}

	err := validateContracts(settings)
	if err != nil {
		return fmt.Errorf(validationErrorFormat, err)
	}

	return nil
}

func geometry(settings Settings) map[string]int {
	return map[string]int{
		"layout.widget_size":   Layout(settings).WidgetSize,
		"layout.review_width":  Layout(settings).ReviewWidth,
		"layout.review_height": Layout(settings).ReviewHeight,
		"layout.review_gap":    Layout(settings).ReviewGap,
	}
}

func validateContracts(settings Settings) error {
	routes := []string{settings.MediaRoutePrefix(), settings.PreviewRoutePrefix()}

	for index := range routes {
		route := routes[index]
		if !validRoute(route) {
			return fmt.Errorf("%w: invalid media route %q", errInvalidConfiguration, route)
		}
	}

	err := validateDistinctValues(settings)
	if err != nil {
		return fmt.Errorf(validationErrorFormat, err)
	}

	return nil
}

func validRoute(route string) bool {
	parsed, err := url.ParseRequestURI(route)
	if err != nil {
		return false
	}

	return routeShape(route) && parsed.Path == route && parsed.RawQuery == ""
}

func validateDistinctValues(settings Settings) error {
	if routesOverlap(settings.MediaRoutePrefix(), settings.PreviewRoutePrefix()) {
		return errOverlappingRoutes
	}

	err := validateRatings(settings)
	if err != nil {
		return fmt.Errorf(validationErrorFormat, err)
	}

	return nil
}

func validateRatings(settings Settings) error {
	// FSRS defines four ratings numbered 1 through 4; configuration must preserve that contract.
	minimumMatches := Library(settings).MinimumRating == fsrsFirstRating
	maximumMatches := Library(settings).MaximumRating == fsrsLastRating

	if !minimumMatches || !maximumMatches {
		return errIncompatibleRatings
	}

	err := validateEvents(settings)
	if err != nil {
		return fmt.Errorf(validationErrorFormat, err)
	}

	return nil
}

func validateEvents(settings Settings) error {
	seen := make(map[string]bool)

	events := []string{
		settings.SchedulesChanged(), settings.ShowReviews(),
		settings.OpenFloatingReview(), settings.CloseFloatingReview(),
	}

	for index := range events {
		event := events[index]
		if seen[event] {
			return fmt.Errorf("%w: event %q is duplicated", errInvalidConfiguration, event)
		}

		seen[event] = true
	}

	return nil
}

func routeShape(route string) bool {
	return strings.HasPrefix(route, routeSeparator) && strings.HasSuffix(route, routeSeparator) &&
		len(route) > len(routeSeparator)
}

func routesOverlap(first, second string) bool {
	return strings.HasPrefix(first, second) || strings.HasPrefix(second, first)
}

// Library projects a validated configuration's independent content rules.
func Library(settings Settings) library.Rules {
	var rules library.Rules

	err := json.Unmarshal(settings.Library(), &rules)
	if err != nil {
		var empty library.Rules

		return empty
	}

	return rules
}

// Layout projects a validated configuration's independent window geometry.
func Layout(settings Settings) companion.Layout {
	var layout companion.Layout

	err := json.Unmarshal(settings.Layout(), &layout)
	if err != nil {
		var empty companion.Layout

		return empty
	}

	return layout
}
