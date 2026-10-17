//go:build !darwin || ios || server

package wails_test

import (
	"testing"

	startupwails "raven/internal/features/autostart/adapters/wails"
)

func TestDetectorClassifiesUnsupportedPlatformsAsManualLaunches(t *testing.T) {
	calls := 0
	startupwails.NewDetector().PrepareLoginDetection(func(automatic bool) {
		calls++
		if automatic {
			t.Fatal("unsupported native detection reported an automatic launch")
		}
	})
	if calls != 1 {
		t.Fatalf("received %d classifications, want 1", calls)
	}
}
