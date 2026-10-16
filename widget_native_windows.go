//go:build windows && !server

package main

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

var (
	widgetUser32       = windows.NewLazySystemDLL("user32.dll")
	widgetSetWindowRgn = widgetUser32.NewProc("SetWindowRgn")
	widgetSetWindowPos = widgetUser32.NewProc("SetWindowPos")
)

func configureNativeWidget(window *application.WebviewWindow) error {
	handle := uintptr(window.NativeWindow())
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
