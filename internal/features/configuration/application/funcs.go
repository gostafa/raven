// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"fmt"

	"raven/internal/features/configuration/ports"
)

// Load obtains the validated application configuration before resources open.
func Load(loader ports.Loader) (ports.Snapshot, error) {
	snapshot, err := loader.Load()
	if err != nil {
		return ports.Snapshot{}, fmt.Errorf("load configuration: %w", err)
	}

	return snapshot, nil
}
