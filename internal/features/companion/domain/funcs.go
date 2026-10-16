// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package domain

// Bounds places the floating review window within the available screen area.
func Bounds(widgetX, widgetY int, area *Rect) Rect {
	width := min(ReviewWidth, area.Width)
	height := min(ReviewHeight, area.Height)
	horizontal := widgetX - ReviewGap - width

	if horizontal < area.X && widgetX+WidgetSize+ReviewGap+width <= area.X+area.Width {
		horizontal = widgetX + WidgetSize + ReviewGap
	}

	return Rect{
		X:     max(area.X, min(horizontal, area.X+area.Width-width)),
		Y:     max(area.Y, min(widgetY+WidgetSize-height, area.Y+area.Height-height)),
		Width: width, Height: height,
	}
}
