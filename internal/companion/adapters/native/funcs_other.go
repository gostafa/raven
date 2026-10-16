//go:build (!windows && !linux) || server

package native

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// macOS applies its transparent background and panel level through window options.
func Configure(_ *application.WebviewWindow) error { return nil }
