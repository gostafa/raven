//go:build windows && !server

package wails

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type widgetProcedureFixture struct {
	result    uintptr
	returned  error
	arguments []uintptr
}

func (procedure *widgetProcedureFixture) Call(arguments ...uintptr) (uintptr, uintptr, error) {
	procedure.arguments = append(procedure.arguments[:0], arguments...)
	return procedure.result, 0, procedure.returned
}

func TestWidgetConfigureWithoutNativeWindow(t *testing.T) {
	if err := NewWidget(nil).Configure(); err != nil {
		t.Fatalf("configure widget without a window: %v", err)
	}
	if err := NewWidget(&application.WebviewWindow{}).Configure(); err != nil {
		t.Fatalf("configure unrealized widget: %v", err)
	}
}

func TestWidgetConfigureNativeCalls(t *testing.T) {
	region := &widgetProcedureFixture{result: 1}
	position := &widgetProcedureFixture{result: 1}
	widget := Widget{setWindowRegion: region, setWindowPosition: position}

	if err := widget.configure(42); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(region.arguments, []uintptr{42, widgetNullRegion, widgetRedrawRegion}) {
		t.Fatalf("unexpected region call: %v", region.arguments)
	}
	if !reflect.DeepEqual(position.arguments, []uintptr{
		42, widgetTopmostWindow, widgetPosition, widgetPosition,
		widgetPosition, widgetPosition, widgetWindowPositionFlags,
	}) {
		t.Fatalf("unexpected position call: %v", position.arguments)
	}
}

func TestWidgetConfigureReportsNativeFailures(t *testing.T) {
	failure := errors.New("native call failed")
	for _, test := range []struct {
		name     string
		region   *widgetProcedureFixture
		position *widgetProcedureFixture
		message  string
	}{
		{
			name:     "region",
			region:   &widgetProcedureFixture{returned: failure},
			position: &widgetProcedureFixture{result: 1},
			message:  "clear widget clipping region",
		},
		{
			name:     "position",
			region:   &widgetProcedureFixture{result: 1},
			position: &widgetProcedureFixture{returned: failure},
			message:  "keep widget on top",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			widget := Widget{setWindowRegion: test.region, setWindowPosition: test.position}
			err := widget.configure(42)
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("unexpected native failure: %v", err)
			}
		})
	}
}
