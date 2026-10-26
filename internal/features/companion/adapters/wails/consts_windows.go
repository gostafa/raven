//go:build windows && !server

package wails

import (
	"syscall"
)

const (
	widgetRedrawRegion        = 1
	widgetWindowPositionFlags = 0x13 // SWP_NOMOVE | SWP_NOSIZE | SWP_NOACTIVATE.
	widgetTopmostWindow       = ^uintptr(0)
	errWidgetNoErrno          = syscall.Errno(zero)
)
