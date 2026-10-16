package companion_test

import (
	"testing"

	"raven/internal/companion"
)

func TestFloatingReviewBounds(t *testing.T) {
	primary := companion.Rect{X: 0, Y: 25, Width: 1440, Height: 850}
	secondary := companion.Rect{X: -1920, Y: -200, Width: 1920, Height: 1080}
	for _, test := range []struct {
		name string
		x, y int
		area companion.Rect
		want companion.Rect
	}{
		{"bottom right prefers left", 1368, 803, primary, companion.Rect{X: 936, Y: 331, Width: 420, Height: 520}},
		{"bottom left flips right", 24, 803, primary, companion.Rect{X: 84, Y: 331, Width: 420, Height: 520}},
		{"top left clamps vertically", 0, 25, primary, companion.Rect{X: 60, Y: 25, Width: 420, Height: 520}},
		{"top right clamps vertically", 1392, 25, primary, companion.Rect{X: 960, Y: 25, Width: 420, Height: 520}},
		{"secondary negative origin", -72, 808, secondary, companion.Rect{X: -504, Y: 336, Width: 420, Height: 520}},
		{"secondary left edge", -1920, -200, secondary, companion.Rect{X: -1860, Y: -200, Width: 420, Height: 520}},
		{"small work area shrinks", 200, 200, companion.Rect{X: 10, Y: 30, Width: 300, Height: 400}, companion.Rect{X: 10, Y: 30, Width: 300, Height: 400}},
		{"neither side fits clamps left", 300, 500, companion.Rect{Width: 700, Height: 800}, companion.Rect{X: 0, Y: 28, Width: 420, Height: 520}},
		{"widget outside work area", 1800, 1100, primary, companion.Rect{X: 1020, Y: 355, Width: 420, Height: 520}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := companion.Bounds(test.x, test.y, &test.area)
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
