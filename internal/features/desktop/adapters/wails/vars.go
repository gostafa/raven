// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"raven/internal/features/desktop/ports"
)

var (
	_ ports.Events           = (*EventPublisher)(nil)
	_ windowEventSource      = (*application.WebviewWindow)(nil)
	_ applicationEventSource = (*application.EventManager)(nil)
)
