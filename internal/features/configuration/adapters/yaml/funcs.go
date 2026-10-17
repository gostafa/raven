// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package yaml

import (
	"fmt"
	"io"
	"strings"

	"go.uber.org/config"

	companion "raven/internal/features/companion/domain"
	"raven/internal/features/configuration/domain"
	"raven/internal/features/configuration/ports"
	library "raven/internal/shared/boltstore/domain"
)

// Load parses and validates the embedded defaults.
func (Loader) Load() (ports.Snapshot, error) {
	settings, err := read(strings.NewReader(appDefaults), strings.NewReader(sharedDefaults))
	if err != nil {
		return ports.Snapshot{}, fmt.Errorf("load embedded configuration: %w", err)
	}

	return ports.Snapshot{Settings: settings}, nil
}

func read(app, shared io.Reader) (*settings, error) {
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

func decode(provider config.Provider) (*settings, error) {
	var decoded settings

	err := provider.Get("").Populate(&decoded.values)
	if err != nil {
		return nil, fmt.Errorf("decode application configuration: %w", err)
	}

	result, err := validated(&decoded)
	if err != nil {
		return nil, fmt.Errorf("validate decoded configuration: %w", err)
	}

	return result, nil
}

func validated(settings *settings) (*settings, error) {
	err := domain.Validate(settings)
	if err != nil {
		return nil, fmt.Errorf("validate application configuration: %w", err)
	}

	return settings, nil
}

func (settings *settings) ApplicationID() string { return settings.values.ApplicationID }

func (settings *settings) LogErrorKey() string { return settings.values.LogErrorKey }

func (settings *settings) LogOperationKey() string { return settings.values.LogOperationKey }

func (settings *settings) MediaRoutePrefix() string { return settings.values.MediaRoutePrefix }

func (settings *settings) PreviewRoutePrefix() string { return settings.values.PreviewRoutePrefix }

func (settings *settings) PreferencesStorageKey() string {
	return settings.values.PreferencesStorageKey
}

func (settings *settings) SchedulesChanged() string { return settings.values.SchedulesChanged }

func (settings *settings) ShowReviews() string { return settings.values.ShowReviews }

func (settings *settings) OpenFloatingReview() string { return settings.values.OpenFloatingReview }

func (settings *settings) CloseFloatingReview() string { return settings.values.CloseFloatingReview }

func (settings *settings) Library() library.Rules { return settings.values.Library }

func (settings *settings) Layout() companion.Layout { return settings.values.Layout }
