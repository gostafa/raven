// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"context"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	"raven/internal/companion"
	widgetnative "raven/internal/companion/adapters/native"
)

// New constructs the adapter with its required runtime dependencies.
func New(app *application.App, handles WindowSet) *Windows {
	return &Windows{
		app:          app,
		mainWindow:   handles.Main,
		widget:       handles.Widget,
		reviewWindow: handles.Review,
	}
}

// ConfigureWidget configures platform-specific floating widget behavior.
func (windows *Windows) ConfigureWidget() {
	application.InvokeSync(func() {
		err := widgetnative.Configure(windows.widget)
		if err != nil {
			windows.app.Logger.ErrorContext(
				context.Background(),
				"could not configure raven widget",
				slog.Any(logErrorKey, err),
			)
		}
	})
}

// ShowMain shows, restores, and focuses the main library window.
func (windows *Windows) ShowMain() {
	windows.mainWindow.Show()
	windows.mainWindow.Restore()
	windows.mainWindow.Focus()
}

// HideMain hides the library and keeps the widget visible.
func (windows *Windows) HideMain() {
	windows.widget.Show()
	windows.mainWindow.Hide()
}

// ResetWidget repositions the widget within the primary screen.
func (windows *Windows) ResetWidget() {
	if screen := windows.app.Screen.GetPrimary(); screen != nil {
		area := screen.WorkArea
		windows.widget.SetPosition(
			area.X+max(zero, area.Width-widgetSize-widgetMargin),
			area.Y+max(zero, area.Height-widgetSize-widgetMargin),
		)
	}

	windows.widget.Show()
	windows.PositionFloatingReview()
}

// Quit is intentionally separate from closing the main window.
func (windows *Windows) Quit() {
	windows.app.Quit()
}

// PositionFloatingReview repositions the visible review window beside the widget.
func (windows *Windows) PositionFloatingReview() {
	if windows.reviewWindow == nil || !windows.reviewWindow.IsVisible() {
		return
	}

	windows.placeFloatingReview()
}

func (windows *Windows) placeFloatingReview() {
	screen, err := windows.widget.GetScreen()
	if err != nil || screen == nil {
		screen = windows.app.Screen.GetPrimary()
	}

	if screen == nil || screen.WorkArea.Width <= zero || screen.WorkArea.Height <= zero {
		return
	}

	horizontal, vertical := windows.widget.Position()
	area := screen.WorkArea
	bounds := companion.Bounds(
		horizontal,
		vertical,
		companion.Rect{X: area.X, Y: area.Y, Width: area.Width, Height: area.Height},
	)
	windows.reviewWindow.SetSize(bounds.Width, bounds.Height)
	windows.reviewWindow.SetPosition(bounds.X, bounds.Y)
}

// RequestFloatingReviewClose asks the frontend to close after pending saves finish.
func (windows *Windows) RequestFloatingReviewClose() {
	// The frontend owns the saving state and ignores close requests until a
	// rating finishes. DispatchWailsEvent targets only this window.
	windows.reviewWindow.DispatchWailsEvent(
		&application.CustomEvent{Name: "raven:close-floating-review", Data: nil, Sender: ""},
	)
}

// ToggleFloatingReview opens the companion card without showing the library.
func (windows *Windows) ToggleFloatingReview() {
	application.InvokeSync(func() {
		if windows.reviewWindow.IsVisible() {
			windows.RequestFloatingReviewClose()

			return
		}

		windows.placeFloatingReview()
		windows.reviewWindow.Show()
		windows.reviewWindow.DispatchWailsEvent(
			&application.CustomEvent{Name: "raven:open-floating-review", Data: nil, Sender: ""},
		)
		windows.reviewWindow.Focus()
	})
}

// HideFloatingReview is called after the review UI has stopped media playback.
func (windows *Windows) HideFloatingReview() {
	windows.reviewWindow.Hide()
}
