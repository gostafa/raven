// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package domain defines floating-window geometry and window ports.
package domain

type (
	// Layout defines floating-window dimensions in logical pixels.
	Layout struct {
		// WidgetSize is the width and height of the floating launcher.
		WidgetSize int `json:"widget_size" yaml:"widget_size"`
		// ReviewWidth is the preferred floating review width.
		ReviewWidth int `json:"review_width" yaml:"review_width"`
		// ReviewHeight is the preferred floating review height.
		ReviewHeight int `json:"review_height" yaml:"review_height"`
		// ReviewGap is the spacing between the launcher and floating review.
		ReviewGap int `json:"review_gap" yaml:"review_gap"`
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
		SyncFloatingReview()
		HideFloatingReview()
	}
)
