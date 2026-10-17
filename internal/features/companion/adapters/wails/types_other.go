//go:build (!windows && !linux) || server

package wails

type (
	// Widget configures the native floating window.
	Widget struct{}
)
