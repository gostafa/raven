//go:build linux && cgo && !android && !server

package native

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

type (
	// Widget configures the native floating window.
	Widget struct{ window *application.WebviewWindow }
)
