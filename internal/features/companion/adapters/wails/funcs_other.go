//go:build (!windows && !linux) || server

package wails

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// NewWidget constructs native configuration for the floating widget.
func NewWidget(_ *application.WebviewWindow) Widget { return Widget{} }

// Configure relies on window options for the transparent background and panel level.
func (Widget) Configure() error { return nil }
