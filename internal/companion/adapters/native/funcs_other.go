//go:build (!windows && !linux) || server

package native

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Configure relies on window options for the transparent background and panel level.
func Configure(_ *application.WebviewWindow) error { return nil }
