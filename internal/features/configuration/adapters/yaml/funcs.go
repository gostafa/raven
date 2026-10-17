// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package yaml

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"go.uber.org/config"

	companion "raven/internal/features/companion/domain"
	"raven/internal/features/configuration/domain"
	library "raven/internal/shared/boltstore/domain"
)

// Load parses and validates the embedded defaults.
func Load() (*Settings, error) {
	settings, err := read(strings.NewReader(appDefaults), strings.NewReader(sharedDefaults))
	if err != nil {
		return nil, fmt.Errorf("load embedded configuration: %w", err)
	}

	return settings, nil
}

func read(app, shared io.Reader) (*Settings, error) {
	provider, err := config.NewYAML(config.Source(app), config.Source(shared))
	if err != nil {
		return nil, fmt.Errorf("parse application configuration: %w", err)
	}

	settings, err := decode(provider)
	if err != nil {
		return nil, fmt.Errorf("read configuration: %w", err)
	}

	return settings, nil
}

func decode(provider config.Provider) (*Settings, error) {
	var decoded Settings

	err := provider.Get("").Populate(&decoded.values)
	if err != nil {
		return nil, fmt.Errorf("decode application configuration: %w", err)
	}

	result, err := validated(provider, &decoded)
	if err != nil {
		return nil, fmt.Errorf("validate decoded configuration: %w", err)
	}

	return result, nil
}

func decodeContracts(provider config.Provider, values *values) error {
	var rules library.Rules

	err := provider.Get("library").Populate(&rules)
	if err != nil {
		return fmt.Errorf("decode library configuration: %w", err)
	}

	values.Library = encodeContract(rules)

	err = decodeLayout(provider, values)
	if err != nil {
		return fmt.Errorf("decode window configuration: %w", err)
	}

	return nil
}

func decodeLayout(provider config.Provider, values *values) error {
	var layout companion.Layout

	err := provider.Get("layout").Populate(&layout)
	if err != nil {
		return fmt.Errorf("decode layout configuration: %w", err)
	}

	values.Layout = encodeContract(layout)

	return nil
}

func validated(provider config.Provider, settings *Settings) (*Settings, error) {
	err := decodeContracts(provider, &settings.values)
	if err != nil {
		return nil, fmt.Errorf("decode configuration contracts: %w", err)
	}

	err = domain.Validate(settings)
	if err != nil {
		return nil, fmt.Errorf("validate application configuration: %w", err)
	}

	return settings, nil
}

// ApplicationID returns the validated ApplicationID configuration value.
func (settings *Settings) ApplicationID() string { return settings.values.ApplicationID }

// LogErrorKey returns the validated LogErrorKey configuration value.
func (settings *Settings) LogErrorKey() string { return settings.values.LogErrorKey }

// LogOperationKey returns the validated LogOperationKey configuration value.
func (settings *Settings) LogOperationKey() string { return settings.values.LogOperationKey }

// MediaRoutePrefix returns the validated MediaRoutePrefix configuration value.
func (settings *Settings) MediaRoutePrefix() string { return settings.values.MediaRoutePrefix }

// PreviewRoutePrefix returns the validated PreviewRoutePrefix configuration value.
func (settings *Settings) PreviewRoutePrefix() string { return settings.values.PreviewRoutePrefix }

// PreferencesStorageKey returns the validated PreferencesStorageKey configuration value.
func (settings *Settings) PreferencesStorageKey() string {
	return settings.values.PreferencesStorageKey
}

// SchedulesChanged returns the validated SchedulesChanged configuration value.
func (settings *Settings) SchedulesChanged() string { return settings.values.SchedulesChanged }

// ShowReviews returns the validated ShowReviews configuration value.
func (settings *Settings) ShowReviews() string { return settings.values.ShowReviews }

// OpenFloatingReview returns the validated OpenFloatingReview configuration value.
func (settings *Settings) OpenFloatingReview() string { return settings.values.OpenFloatingReview }

// CloseFloatingReview returns the validated CloseFloatingReview configuration value.
func (settings *Settings) CloseFloatingReview() string { return settings.values.CloseFloatingReview }

// Library returns the validated Library configuration value.
func (settings *Settings) Library() json.RawMessage { return contractBytes(settings.values.Library) }

// Layout returns the validated Layout configuration value.
func (settings *Settings) Layout() json.RawMessage { return contractBytes(settings.values.Layout) }

func encodeContract[Value any](value Value) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}

	return encoded
}

func contractBytes(value any) json.RawMessage {
	encoded, ok := value.(json.RawMessage)
	if !ok {
		return nil
	}

	return slices.Clone(encoded)
}
