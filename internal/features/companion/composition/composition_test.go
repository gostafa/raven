package composition

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"raven/internal/features/companion/adapters/wails"
	configyaml "raven/internal/features/configuration/adapters/yaml"
)

func TestWindowProvidersRetainConfiguration(t *testing.T) {
	settings, err := configyaml.Load()
	if err != nil {
		t.Fatal(err)
	}
	handles := wails.WindowSet{Widget: &application.WebviewWindow{}}
	configuration := newConfiguration(handles)
	windows := newWindows(
		windowParams{
			App:           application.New(application.Options{}),
			Handles:       handles,
			Configuration: configuration,
		},
		settings,
	)
	if windows == nil {
		t.Fatal("window provider returned no controller")
	}
}
