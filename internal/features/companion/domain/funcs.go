// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package domain

// Bounds places the floating review window within the available screen area.
func (layout *Layout) Bounds(widgetX, widgetY int, area *Rect) Rect {
	width := min(layout.ReviewWidth, area.Width)
	height := min(layout.ReviewHeight, area.Height)
	horizontal := widgetX - layout.ReviewGap - width
	right := widgetX + layout.WidgetSize + layout.ReviewGap

	if horizontal < area.X && right+width <= area.X+area.Width {
		horizontal = right
	}

	return Rect{
		X:     max(area.X, min(horizontal, area.X+area.Width-width)),
		Y:     max(area.Y, min(widgetY+layout.WidgetSize-height, area.Y+area.Height-height)),
		Width: width, Height: height,
	}
}
