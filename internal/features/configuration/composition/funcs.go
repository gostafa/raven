// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"fmt"

	"raven/internal/features/configuration/adapters/yaml"
	"raven/internal/features/configuration/domain"
	"raven/internal/features/configuration/ports"
)

//nolint:ireturn // Composition binds the concrete adapter to the consumer-owned port.
func newLoader() ports.Loader { return yaml.Loader{} }

func load(loader ports.Loader) (*domain.Settings, error) {
	settings, err := loader.Load()
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}

	return settings, nil
}
