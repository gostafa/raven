package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestFloatingReviewBounds(t *testing.T) {
	primary := application.Rect{X: 0, Y: 25, Width: 1440, Height: 850}
	secondary := application.Rect{X: -1920, Y: -200, Width: 1920, Height: 1080}
	for _, test := range []struct {
		name string
		x, y int
		area application.Rect
		want application.Rect
	}{
		{"bottom right prefers left", 1368, 803, primary, application.Rect{X: 936, Y: 331, Width: 420, Height: 520}},
		{"bottom left flips right", 24, 803, primary, application.Rect{X: 84, Y: 331, Width: 420, Height: 520}},
		{"top left clamps vertically", 0, 25, primary, application.Rect{X: 60, Y: 25, Width: 420, Height: 520}},
		{"top right clamps vertically", 1392, 25, primary, application.Rect{X: 960, Y: 25, Width: 420, Height: 520}},
		{"secondary negative origin", -72, 808, secondary, application.Rect{X: -504, Y: 336, Width: 420, Height: 520}},
		{"secondary left edge", -1920, -200, secondary, application.Rect{X: -1860, Y: -200, Width: 420, Height: 520}},
		{"small work area shrinks", 200, 200, application.Rect{X: 10, Y: 30, Width: 300, Height: 400}, application.Rect{X: 10, Y: 30, Width: 300, Height: 400}},
		{"neither side fits clamps left", 300, 500, application.Rect{Width: 700, Height: 800}, application.Rect{X: 0, Y: 28, Width: 420, Height: 520}},
		{"widget outside work area", 1800, 1100, primary, application.Rect{X: 1020, Y: 355, Width: 420, Height: 520}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := floatingReviewBounds(test.x, test.y, test.area)
			if got != test.want {
				t.Fatalf("got %+v, want %+v", got, test.want)
			}
			if got.X < test.area.X || got.Y < test.area.Y ||
				got.X+got.Width > test.area.X+test.area.Width ||
				got.Y+got.Height > test.area.Y+test.area.Height {
				t.Fatalf("panel %+v escapes work area %+v", got, test.area)
			}
		})
	}
}
