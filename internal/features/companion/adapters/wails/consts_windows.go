//go:build windows && !server

package wails

const (
	widgetRedrawRegion        = 1
	widgetWindowPositionFlags = 0x13 // SWP_NOMOVE | SWP_NOSIZE | SWP_NOACTIVATE.
	widgetTopmostWindow       = ^uintptr(0)
)
