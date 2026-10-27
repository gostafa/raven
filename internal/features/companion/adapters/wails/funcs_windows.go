//go:build windows && !server

package wails

import (
	"fmt"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

// NewWidget constructs native configuration for the floating widget.
func NewWidget(window *application.WebviewWindow) Widget {
	user32 := windows.NewLazySystemDLL("user32.dll")

	return widgetWithNativeProcedures(
		func() uintptr {
			if window == nil {
				return zero
			}

			return uintptr(window.NativeWindow())
		},
		user32.NewProc("SetWindowRgn"),
		user32.NewProc("SetWindowPos"),
	)
}

// Configure applies native clipping and stacking behavior to the floating widget.
func (widget Widget) Configure() error {
	if widget.configure == nil {
		return nil
	}

	err := widget.configure()
	if err != nil {
		return fmt.Errorf("configure widget: %w", err)
	}

	return nil
}

func widgetWithNativeProcedures(
	nativeHandle func() uintptr,
	setWindowRegion, setWindowPosition widgetProcedure,
) Widget {
	return Widget{configure: func() error {
		handle := nativeHandle()
		if handle == zero {
			return nil
		}

		err := configureNativeWindow(handle, setWindowRegion, setWindowPosition)
		if err != nil {
			return fmt.Errorf("configure widget native operations: %w", err)
		}

		return nil
	}}
}

func configureNativeWindow(
	handle uintptr,
	setWindowRegion, setWindowPosition widgetProcedure,
) error {
	err := clearWidgetRegion(handle, setWindowRegion)
	if err != nil {
		return fmt.Errorf("configure widget region: %w", err)
	}

	err = keepWidgetOnTop(handle, setWindowPosition)
	if err != nil {
		return fmt.Errorf("configure widget position: %w", err)
	}

	return nil
}

func clearWidgetRegion(handle uintptr, procedure widgetProcedure) error {
	call := callWidgetProcedure(procedure, handle, zero, widgetRedrawRegion)
	if call.result == zero {
		return fmt.Errorf("clear widget clipping region: %w", call.err)
	}

	return nil
}

func keepWidgetOnTop(handle uintptr, procedure widgetProcedure) error {
	call := callWidgetProcedure(
		procedure,
		handle,
		widgetTopmostWindow,
		zero,
		zero,
		zero,
		zero,
		widgetWindowPositionFlags,
	)
	if call.result == zero {
		return fmt.Errorf("keep widget on top: %w", call.err)
	}

	return nil
}

func callWidgetProcedure(procedure widgetProcedure, arguments ...uintptr) widgetProcedureCall {
	result, secondResult, err := procedure.Call(arguments...)
	return widgetProcedureResult(result, secondResult, err)
}

func widgetProcedureResult(result, secondResult uintptr, err error) widgetProcedureCall {
	if result != zero {
		return widgetProcedureCall{err: nil, secondResult: secondResult, result: result}
	}
	if err == nil || err == syscall.Errno(zero) {
		err = errWidgetProcedureReturnedZero
	}
	return widgetProcedureCall{
		err:          fmt.Errorf("call native widget procedure: %w", err),
		secondResult: secondResult,
		result:       result,
	}
}
