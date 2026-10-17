//go:build windows && !server

package wails

import (
	"golang.org/x/sys/windows"
)

var (
	widgetUser32       = windows.NewLazySystemDLL("user32.dll")
	widgetSetWindowRgn = widgetUser32.NewProc("SetWindowRgn")
	widgetSetWindowPos = widgetUser32.NewProc("SetWindowPos")
)
