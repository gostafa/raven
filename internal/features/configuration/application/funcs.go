// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"fmt"

	"raven/internal/features/configuration/domain"
)

// Load obtains the validated application configuration before resources open.
func Load[Value domain.Settings](loader func() (Value, error)) (Value, error) {
	snapshot, err := loader()
	if err != nil {
		var empty Value

		return empty, fmt.Errorf("load configuration: %w", err)
	}

	return snapshot, nil
}
