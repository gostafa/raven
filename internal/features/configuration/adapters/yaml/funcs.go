// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package yaml

import (
	"fmt"
	"io"
	"strings"

	"go.uber.org/config"

	"raven/internal/features/configuration/domain"
)

// Load parses and validates the embedded defaults.
func (Loader) Load() (*domain.Settings, error) {
	settings, err := read(strings.NewReader(appDefaults), strings.NewReader(sharedDefaults))
	if err != nil {
		return nil, fmt.Errorf("load embedded configuration: %w", err)
	}

	return settings, nil
}

func read(app, shared io.Reader) (*domain.Settings, error) {
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

func decode(provider config.Provider) (*domain.Settings, error) {
	var settings domain.Settings

	err := provider.Get("").Populate(&settings)
	if err != nil {
		return nil, fmt.Errorf("decode application configuration: %w", err)
	}

	result, err := validated(&settings)
	if err != nil {
		return nil, fmt.Errorf("validate decoded configuration: %w", err)
	}

	return result, nil
}

func validated(settings *domain.Settings) (*domain.Settings, error) {
	err := settings.Validate()
	if err != nil {
		return nil, fmt.Errorf("validate application configuration: %w", err)
	}

	return settings, nil
}
