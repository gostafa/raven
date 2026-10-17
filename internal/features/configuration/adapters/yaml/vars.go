// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package yaml

import (
	_ "embed"

	"raven/internal/features/configuration/ports"
)

var (
	_ ports.Loader = Loader{}
	//go:embed app.yml
	appDefaults string
	//go:embed shared.yml
	sharedDefaults string
)
