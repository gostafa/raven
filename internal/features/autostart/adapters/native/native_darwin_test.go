//go:build darwin && !ios && !server

package native

import (
	"reflect"
	"testing"
)

func TestDetectorForwardsNativeLoginClassification(t *testing.T) {
	var detected []bool
	New().PrepareLoginDetection(func(automatic bool) { detected = append(detected, automatic) })
	t.Cleanup(func() { nativeLoginDetected = func(bool) {} })
	ravenLoginLaunchDetected(0)
	ravenLoginLaunchDetected(1)
	if !reflect.DeepEqual(detected, []bool{false, true}) {
		t.Fatalf("unexpected native launch classifications: %v", detected)
	}
}
