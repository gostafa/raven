package domain_test

import (
	"image"
	"testing"

	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
)

func TestFloatingReviewBounds(t *testing.T) {
	primary := image.Rect(0, 25, 1440, 875)
	secondary := image.Rect(-1920, -200, 0, 880)
	for _, test := range []struct {
		name string
		x, y int
		area image.Rectangle
		want image.Rectangle
	}{
		{"bottom right prefers left", 1368, 803, primary, image.Rect(996, 239, 1356, 851)},
		{"bottom left flips right", 24, 803, primary, image.Rect(84, 239, 444, 851)},
		{"top left clamps vertically", 0, 25, primary, image.Rect(60, 25, 420, 637)},
		{"top right clamps vertically", 1392, 25, primary, image.Rect(1020, 25, 1380, 637)},
		{"secondary negative origin", -72, 808, secondary, image.Rect(-444, 244, -84, 856)},
		{"secondary left edge", -1920, -200, secondary, image.Rect(-1860, -200, -1500, 412)},
		{"small work area shrinks", 200, 200, image.Rect(10, 30, 310, 430), image.Rect(10, 30, 310, 430)},
		{"neither side fits clamps left", 300, 500, image.Rect(0, 0, 700, 800), image.Rect(0, 0, 360, 612)},
		{"widget outside work area", 1800, 1100, primary, image.Rect(1080, 263, 1440, 875)},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := new(
				configuration.Layout(configurationFixture()),
			).Bounds(test.x, test.y, &test.area)
			if got != test.want {
				t.Fatalf("got %+v, want %+v", got, test.want)
			}
			if !got.In(test.area) {
				t.Fatalf("panel %+v escapes work area %+v", got, test.area)
			}
		})
	}
}

func configurationFixture() configuration.Settings {
	settings, err := configyaml.Load()
	if err != nil {
		panic(err)
	}
	return settings
}
