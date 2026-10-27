//go:build windows && !server

package wails

type (
	// Widget configures the native floating window.
	Widget struct{ configure func() error }

	widgetProcedureCall struct {
		err          error
		secondResult uintptr
		result       uintptr
	}

	widgetProcedure interface {
		Call(arguments ...uintptr) (uintptr, uintptr, error)
	}
)
