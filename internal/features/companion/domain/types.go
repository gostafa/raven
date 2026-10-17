// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package domain defines floating-window geometry and window ports.
package domain

type (
	// Layout defines floating-window dimensions in logical pixels.
	Layout struct {
		WidgetSize   int `yaml:"widget_size"`
		ReviewWidth  int `yaml:"review_width"`
		ReviewHeight int `yaml:"review_height"`
		ReviewGap    int `yaml:"review_gap"`
	}

	// Rect describes a desktop rectangle in logical pixels.
	Rect struct {
		// X is the horizontal origin in logical pixels.
		X int
		// Y is the vertical origin in logical pixels.
		Y int
		// Width is the horizontal extent in logical pixels.
		Width int
		// Height is the vertical extent in logical pixels.
		Height int
	}

	// Windows controls Raven main, widget, and floating review windows.
	Windows interface {
		mainWindow
		Widget
		FloatingReview
	}

	// mainWindow controls the library window and application lifetime.
	mainWindow interface {
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
