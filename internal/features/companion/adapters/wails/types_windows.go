//go:build windows && !server

package wails

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

type (
	// Widget configures the native floating window.
	Widget struct {
		window            *application.WebviewWindow
		setWindowRegion   widgetProcedure
		setWindowPosition widgetProcedure
	}

	widgetProcedure interface {
		Call(...uintptr) (uintptr, uintptr, error)
	}
)
