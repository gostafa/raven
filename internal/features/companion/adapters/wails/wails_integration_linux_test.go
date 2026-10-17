//go:build linux && cgo && !android && !server

package wails_test

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	companionwails "raven/internal/features/companion/adapters/wails"
)

func TestConfigurationBeforeNativeWindowCreation(t *testing.T) {
	widget := companionwails.NewWidget(&application.WebviewWindow{})
	if err := widget.Configure(); err != nil {
		t.Fatalf("unrealized widget: %v", err)
	}
}
