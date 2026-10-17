//go:build linux && cgo && !android && !server

package native_test

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"raven/internal/features/companion/adapters/native"
)

func TestConfigurationBeforeNativeWindowCreation(t *testing.T) {
	widget := native.New(&application.WebviewWindow{})
	if err := widget.Configure(); err != nil {
		t.Fatalf("unrealized widget: %v", err)
	}
}
