package wails

import (
	"errors"
	"reflect"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"raven/internal/autostart"
)

type fakeAutostart struct {
	options application.AutostartOptions
	enabled bool
	failure error
}

func (native *fakeAutostart) EnableWithOptions(options application.AutostartOptions) error {
	native.options = options
	native.enabled = true
	return native.failure
}

func (native *fakeAutostart) Disable() error           { native.enabled = false; return native.failure }
func (native *fakeAutostart) IsEnabled() (bool, error) { return native.enabled, native.failure }

func TestBackendTranslatesOptionsAndPreservesFailures(t *testing.T) {
	native := &fakeAutostart{}
	backend := New(native)
	options := autostart.Options{Identifier: "raven", Arguments: []string{"--autostart"}}
	if err := backend.EnableWithOptions(&options); err != nil {
		t.Fatal(err)
	}
	if native.options.Identifier != options.Identifier ||
		!reflect.DeepEqual(native.options.Arguments, options.Arguments) {
		t.Fatalf("lost options: %+v", native.options)
	}
	if enabled, err := backend.IsEnabled(); err != nil || !enabled {
		t.Fatalf("enabled=%v error=%v", enabled, err)
	}
	if err := backend.Disable(); err != nil {
		t.Fatal(err)
	}
	if enabled, err := backend.IsEnabled(); err != nil || enabled {
		t.Fatalf("enabled=%v error=%v", enabled, err)
	}
	failure := errors.New("native registration failed")
	native.failure = failure
	if err := backend.EnableWithOptions(&options); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := backend.Disable(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := backend.IsEnabled(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
