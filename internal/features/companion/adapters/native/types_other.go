//go:build (!windows && !linux) || server

package native

type (
	// Widget configures the native floating window.
	Widget struct{}
)
