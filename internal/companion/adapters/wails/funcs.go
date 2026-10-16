// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"raven/internal/companion"
	widgetnative "raven/internal/companion/adapters/native"
)

func New(app *application.App, main, widget, review *application.WebviewWindow) companion.Windows {
	return &windows{app: app, mainWindow: main, widget: widget, reviewWindow: review}
}

func (r *windows) ConfigureWidget() {
	application.InvokeSync(func() {
		err := widgetnative.Configure(r.widget)
		if err != nil {
			r.app.Logger.Error("Could not configure Raven widget", "error", err)
		}
	})
}

func (r *windows) ShowMain() {
	r.mainWindow.Show()
	r.mainWindow.Restore()
	r.mainWindow.Focus()
}

func (r *windows) HideMain() {
	r.widget.Show()
	r.mainWindow.Hide()
}

func (r *windows) ResetWidget() {
	if screen := r.app.Screen.GetPrimary(); screen != nil {
		area := screen.WorkArea
		r.widget.SetPosition(
			area.X+max(0, area.Width-widgetSize-24),
			area.Y+max(0, area.Height-widgetSize-24),
		)
	}

	r.widget.Show()
	r.PositionFloatingReview()
}

// Quit is intentionally separate from closing the main window.
func (r *windows) Quit() {
	r.app.Quit()
}

// ShowReviews opens the main window and requests a review session.
func (r *windows) PositionFloatingReview() {
	if r.reviewWindow == nil || !r.reviewWindow.IsVisible() {
		return
	}

	r.placeFloatingReview()
}

func (r *windows) placeFloatingReview() {
	screen, err := r.widget.GetScreen()
	if err != nil || screen == nil {
		screen = r.app.Screen.GetPrimary()
	}

	if screen == nil || screen.WorkArea.Width <= 0 || screen.WorkArea.Height <= 0 {
		return
	}

	x, y := r.widget.Position()
	area := screen.WorkArea
	bounds := companion.Bounds(
		x,
		y,
		companion.Rect{X: area.X, Y: area.Y, Width: area.Width, Height: area.Height},
	)
	r.reviewWindow.SetSize(bounds.Width, bounds.Height)
	r.reviewWindow.SetPosition(bounds.X, bounds.Y)
}

func (r *windows) RequestFloatingReviewClose() {
	// The frontend owns the saving state and ignores close requests until a
	// rating finishes. DispatchWailsEvent targets only this window.
	r.reviewWindow.DispatchWailsEvent(&application.CustomEvent{Name: "raven:close-floating-review"})
}

// ToggleFloatingReview opens the companion card without showing the library.
func (r *windows) ToggleFloatingReview() {
	application.InvokeSync(func() {
		if r.reviewWindow.IsVisible() {
			r.RequestFloatingReviewClose()

			return
		}

		r.placeFloatingReview()
		r.reviewWindow.Show()
		r.reviewWindow.DispatchWailsEvent(
			&application.CustomEvent{Name: "raven:open-floating-review"},
		)
		r.reviewWindow.Focus()
	})
}

// HideFloatingReview is called after the review UI has stopped media playback.
func (r *windows) HideFloatingReview() {
	r.reviewWindow.Hide()
}
