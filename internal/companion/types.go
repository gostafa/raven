// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package companion defines floating-window geometry and window ports.
package companion

type (
	// Rect describes a desktop rectangle in logical pixels.
	Rect struct{ X, Y, Width, Height int }

	// Windows controls Raven main, widget, and floating review windows.
	Windows interface {
		MainWindow
		Widget
		FloatingReview
	}

	// MainWindow controls the library window and application lifetime.
	MainWindow interface {
		ShowMain()
		HideMain()
		Quit()
	}

	// Widget controls floating widget setup and placement.
	Widget interface {
		ConfigureWidget()
		ResetWidget()
	}

	// FloatingReview controls the separate review window.
	FloatingReview interface {
		PositionFloatingReview()
		RequestFloatingReviewClose()
		ToggleFloatingReview()
		HideFloatingReview()
	}
)
