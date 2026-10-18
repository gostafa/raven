// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"fmt"

	"go.uber.org/fx"

	"raven/internal/features/configuration/adapters/yaml"
	"raven/internal/features/configuration/application"
)

func newLoader() func() (*yaml.Settings, error) { return yaml.Load }

func load(loader func() (*yaml.Settings, error)) (settingsResult, error) {
	snapshot, err := application.Load(loader)
	if err != nil {
		return settingsResult{}, fmt.Errorf("provide configuration: %w", err)
	}

	return settingsResult{Out: fx.Out{}, Settings: snapshot}, nil
}
