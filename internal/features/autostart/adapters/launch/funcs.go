// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package launch

import autostart "raven/internal/features/autostart/application"

// New creates a coalescing queue that is safe before native windows exist.
func New() *Requests {
	requests := &Requests{pending: make(chan struct{}, 1)}

	requests.visibility = autostart.NewVisibility(requests.request)

	return requests
}

func (requests *Requests) request() {
	select {
	case requests.pending <- struct{}{}:
	default:
	}
}

// Visibility returns launch policy shared with the desktop adapter.
func (requests *Requests) Visibility() *autostart.Visibility { return requests.visibility }

// Pending returns coalesced requests to show the main window.
func (requests *Requests) Pending() <-chan struct{} { return requests.pending }
