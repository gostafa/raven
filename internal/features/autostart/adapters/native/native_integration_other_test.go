//go:build !darwin || ios || server

package native_test

import (
	"testing"

	"raven/internal/features/autostart/adapters/native"
)

func TestDetectorClassifiesUnsupportedPlatformsAsManualLaunches(t *testing.T) {
	calls := 0
	native.New().PrepareLoginDetection(func(automatic bool) {
		calls++
		if automatic {
			t.Fatal("unsupported native detection reported an automatic launch")
		}
	})
	if calls != 1 {
		t.Fatalf("received %d classifications, want 1", calls)
	}
}
