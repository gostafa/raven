//go:build (!windows && !linux) || server

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// macOS applies its transparent background and panel level through window options.
func configureNativeWidget(_ *application.WebviewWindow) error { return nil }
