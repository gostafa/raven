//go:build windows && !server

package native

import (
	"golang.org/x/sys/windows"
)

var (
	widgetUser32       = windows.NewLazySystemDLL("user32.dll")
	widgetSetWindowRgn = widgetUser32.NewProc("SetWindowRgn")
	widgetSetWindowPos = widgetUser32.NewProc("SetWindowPos")
)

var (
	widgetUser32       = windows.NewLazySystemDLL("user32.dll")
	widgetSetWindowRgn = widgetUser32.NewProc("SetWindowRgn")
	widgetSetWindowPos = widgetUser32.NewProc("SetWindowPos")
)
