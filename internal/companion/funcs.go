// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package companion

func Bounds(widgetX, widgetY int, area Rect) Rect {
	width := min(ReviewWidth, area.Width)
	height := min(ReviewHeight, area.Height)
	x := widgetX - ReviewGap - width

	if x < area.X && widgetX+WidgetSize+ReviewGap+width <= area.X+area.Width {
		x = widgetX + WidgetSize + ReviewGap
	}

	return Rect{
		X:     max(area.X, min(x, area.X+area.Width-width)),
		Y:     max(area.Y, min(widgetY+WidgetSize-height, area.Y+area.Height-height)),
		Width: width, Height: height,
	}
}
