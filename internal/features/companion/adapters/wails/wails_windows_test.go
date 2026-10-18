//go:build windows && !server

package wails

import (
	"errors"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type widgetProcedureFixture struct {
	result    uintptr
	lastError uintptr
	returned  error
	arguments []uintptr
}

func (procedure *widgetProcedureFixture) Call(arguments ...uintptr) (uintptr, uintptr, error) {
	procedure.arguments = append(procedure.arguments[:0], arguments...)
	return procedure.result, procedure.lastError, procedure.returned
}

func TestWidgetConfigureWithoutNativeWindow(t *testing.T) {
	if err := (Widget{}).Configure(); err != nil {
		t.Fatalf("configure an empty widget: %v", err)
	}
	if err := NewWidget(nil).Configure(); err != nil {
		t.Fatalf("configure widget without a window: %v", err)
	}
	if err := NewWidget(&application.WebviewWindow{}).Configure(); err != nil {
		t.Fatalf("configure unrealized widget: %v", err)
	}
}

func TestWidgetConfigureWrapsNativeFailure(t *testing.T) {
	failure := errors.New("native call failed")
	widget := widgetWithNativeProcedures(
		func() uintptr { return 42 },
		&widgetProcedureFixture{returned: failure},
		&widgetProcedureFixture{result: 1},
	)

	err := widget.Configure()
	if !errors.Is(err, failure) ||
		!strings.Contains(err.Error(), "configure widget") {
		t.Fatalf("unexpected widget configuration error: %v", err)
	}
}

func TestWidgetConfigureWithNativeWindow(t *testing.T) {
	region := &widgetProcedureFixture{result: 1}
	position := &widgetProcedureFixture{result: 1}
	widget := widgetWithNativeProcedures(
		func() uintptr { return 42 },
		region,
		position,
	)

	if err := widget.Configure(); err != nil {
		t.Fatal(err)
	}
	if len(region.arguments) == 0 || len(position.arguments) == 0 {
		t.Fatalf(
			"native procedures were not called: region=%v position=%v",
			region.arguments,
			position.arguments,
		)
	}
}

func TestWidgetConfigureNativeCalls(t *testing.T) {
	region := &widgetProcedureFixture{result: 1}
	position := &widgetProcedureFixture{result: 1}
	if err := configureNativeWindow(42, region, position); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(region.arguments, []uintptr{42, zero, widgetRedrawRegion}) {
		t.Fatalf("unexpected region call: %v", region.arguments)
	}
	if !reflect.DeepEqual(position.arguments, []uintptr{
		42, widgetTopmostWindow, zero, zero, zero, zero, widgetWindowPositionFlags,
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
			err := configureNativeWindow(42, test.region, test.position)
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("unexpected native failure: %v", err)
			}
		})
	}
}

func TestWidgetProcedureFailureUsesLastError(t *testing.T) {
	for _, test := range []struct {
		name      string
		lastError uintptr
		wantError error
	}{
		{name: "missing last error", wantError: errWidgetProcedureReturnedZero},
		{name: "reported last error", lastError: 5, wantError: syscall.Errno(5)},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := callWidgetProcedure(&widgetProcedureFixture{lastError: test.lastError})
			if call.result != zero || call.err == nil {
				t.Fatalf("expected native procedure failure, got %+v", call)
			}
			if test.wantError != nil && !errors.Is(call.err, test.wantError) {
				t.Fatalf("unexpected native procedure error: %v", call.err)
			}
		})
	}
}
