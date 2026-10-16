// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package launch

import autostart "raven/internal/features/autostart/application"

type (
	// Requests decouples launch classification from native window construction.
	Requests struct {
		visibility *autostart.Visibility
		pending    chan struct{}
	}
)
