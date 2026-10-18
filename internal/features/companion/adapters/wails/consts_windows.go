//go:build windows && !server

package wails

const (
	widgetNullRegion          = 0
	widgetRedrawRegion        = 1
	widgetPosition            = 0
	widgetWindowPositionFlags = 0x13 // SWP_NOMOVE | SWP_NOSIZE | SWP_NOACTIVATE.
	widgetTopmostWindow       = ^uintptr(0)
)
