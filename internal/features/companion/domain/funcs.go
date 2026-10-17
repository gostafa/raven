// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package domain

import (
	"image"
)

// Bounds places the floating review window within the available screen area.
func (layout *Layout) Bounds(widgetX, widgetY int, area *image.Rectangle) image.Rectangle {
	width := min(layout.ReviewWidth, area.Dx())
	height := min(layout.ReviewHeight, area.Dy())
	horizontal := widgetX - layout.ReviewGap - width
	right := widgetX + layout.WidgetSize + layout.ReviewGap

	if horizontal < area.Min.X && right+width <= area.Max.X {
		horizontal = right
	}

	horizontal = max(area.Min.X, min(horizontal, area.Max.X-width))

	vertical := max(area.Min.Y, min(widgetY+layout.WidgetSize-height, area.Max.Y-height))

	return image.Rect(horizontal, vertical, horizontal+width, vertical+height)
}
