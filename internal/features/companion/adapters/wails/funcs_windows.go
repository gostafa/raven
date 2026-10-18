//go:build windows && !server

package wails

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

// NewWidget constructs native configuration for the floating widget.
func NewWidget(window *application.WebviewWindow) Widget {
	user32 := windows.NewLazySystemDLL("user32.dll")
	return Widget{
		window:            window,
		setWindowRegion:   user32.NewProc("SetWindowRgn"),
		setWindowPosition: user32.NewProc("SetWindowPos"),
	}
}

// Configure applies native clipping and stacking behavior to the floating widget.
func (widget Widget) Configure() error {
	if widget.window == nil {
		return nil
	}

	handle := uintptr(widget.window.NativeWindow())
	if handle == 0 {
		return nil
	}

	return widget.configure(handle)
}

func (widget Widget) configure(handle uintptr) error {
	// Let the transparent webview draw the circle and the overlapping badge.
	if ok, _, err := widget.setWindowRegion.Call(
		handle,
		widgetNullRegion,
		widgetRedrawRegion,
	); ok == 0 {
		return fmt.Errorf("clear widget clipping region: %w", err)
	}
	if ok, _, err := widget.setWindowPosition.Call(
		handle,
		widgetTopmostWindow,
		widgetPosition,
		widgetPosition,
		widgetPosition,
		widgetPosition,
		widgetWindowPositionFlags,
	); ok == 0 {
		return fmt.Errorf("keep widget on top: %w", err)
	}

	return nil
}
