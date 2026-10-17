//go:build windows && !server

package wails

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// NewWidget constructs native configuration for the floating widget.
func NewWidget(window *application.WebviewWindow) Widget { return Widget{window: window} }

func (widget Widget) Configure() error {
	handle := uintptr(widget.window.NativeWindow())
	if handle == 0 {
		return nil
	}
	// Let the transparent webview draw the circle and the overlapping badge.
	if ok, _, err := widgetSetWindowRgn.Call(handle, 0, 1); ok == 0 {
		return fmt.Errorf("clear widget clipping region: %w", err)
	}
	// HWND_TOPMOST, SWP_NOMOVE | SWP_NOSIZE | SWP_NOACTIVATE.
	if ok, _, err := widgetSetWindowPos.Call(handle, ^uintptr(0), 0, 0, 0, 0, 0x13); ok == 0 {
		return fmt.Errorf("keep widget on top: %w", err)
	}
	return nil
}
