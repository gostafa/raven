package main

import "github.com/wailsapp/wails/v3/pkg/application"

const (
	reviewWidth  = 420
	reviewHeight = 520
	reviewGap    = 12
)

// floatingReviewBounds uses logical screen coordinates, including negative
// coordinates on secondary displays. Small work areas shrink the panel.
func floatingReviewBounds(widgetX, widgetY int, area application.Rect) application.Rect {
	width := min(reviewWidth, area.Width)
	height := min(reviewHeight, area.Height)
	x := widgetX - reviewGap - width
	if x < area.X && widgetX+widgetSize+reviewGap+width <= area.X+area.Width {
		x = widgetX + widgetSize + reviewGap
	}
	return application.Rect{
		X:     max(area.X, min(x, area.X+area.Width-width)),
		Y:     max(area.Y, min(widgetY+widgetSize-height, area.Y+area.Height-height)),
		Width: width, Height: height,
	}
}

func (r *RavenService) positionFloatingReview() {
	if r.reviewWindow == nil || !r.reviewWindow.IsVisible() {
		return
	}
	r.placeFloatingReview()
}

func (r *RavenService) placeFloatingReview() {
	screen, err := r.widget.GetScreen()
	if err != nil || screen == nil {
		screen = r.app.Screen.GetPrimary()
	}
	if screen == nil || screen.WorkArea.Width <= 0 || screen.WorkArea.Height <= 0 {
		return
	}
	x, y := r.widget.Position()
	bounds := floatingReviewBounds(x, y, screen.WorkArea)
	r.reviewWindow.SetSize(bounds.Width, bounds.Height)
	r.reviewWindow.SetPosition(bounds.X, bounds.Y)
}

func (r *RavenService) requestFloatingReviewClose() {
	// The frontend owns the saving state and ignores close requests until a
	// rating finishes. DispatchWailsEvent targets only this window.
	r.reviewWindow.DispatchWailsEvent(&application.CustomEvent{Name: "raven:close-floating-review"})
}

// ToggleFloatingReview opens the companion card without showing the library.
func (r *RavenService) ToggleFloatingReview() {
	application.InvokeSync(func() {
		if r.reviewWindow.IsVisible() {
			r.requestFloatingReviewClose()
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
func (r *RavenService) HideFloatingReview() {
	r.reviewWindow.Hide()
}
