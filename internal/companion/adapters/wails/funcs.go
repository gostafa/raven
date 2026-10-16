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

func New(app *application.App, main, widget, review *application.WebviewWindow) *Windows {
	return &Windows{app: app, mainWindow: main, widget: widget, reviewWindow: review}
}

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

func (windows *Windows) ShowMain() {
	windows.mainWindow.Show()
	windows.mainWindow.Restore()
	windows.mainWindow.Focus()
}

func (windows *Windows) HideMain() {
	windows.widget.Show()
	windows.mainWindow.Hide()
}

func (windows *Windows) ResetWidget() {
	if screen := windows.app.Screen.GetPrimary(); screen != nil {
		area := screen.WorkArea
		windows.widget.SetPosition(
			area.X+max(0, area.Width-widgetSize-24),
			area.Y+max(0, area.Height-widgetSize-24),
		)
	}

	windows.widget.Show()
	windows.PositionFloatingReview()
}

// Quit is intentionally separate from closing the main window.
func (windows *Windows) Quit() {
	windows.app.Quit()
}

// ShowReviews opens the main window and requests a review session.
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

	if screen == nil || screen.WorkArea.Width <= 0 || screen.WorkArea.Height <= 0 {
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

func (windows *Windows) RequestFloatingReviewClose() {
	// The frontend owns the saving state and ignores close requests until a
	// rating finishes. DispatchWailsEvent targets only this window.
	windows.reviewWindow.DispatchWailsEvent(
		&application.CustomEvent{Name: "raven:close-floating-review"},
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
			&application.CustomEvent{Name: "raven:open-floating-review"},
		)
		windows.reviewWindow.Focus()
	})
}

// HideFloatingReview is called after the review UI has stopped media playback.
func (windows *Windows) HideFloatingReview() {
	windows.reviewWindow.Hide()
}
