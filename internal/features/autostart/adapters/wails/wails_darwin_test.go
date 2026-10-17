//go:build darwin && !ios && !server

package wails

import (
	"reflect"
	"runtime/cgo"
	"testing"
)

func TestPreparingDetectionWaitsForNativeLaunch(t *testing.T) {
	called := false
	NewDetector().PrepareLoginDetection(func(bool) { called = true })
	if called {
		t.Fatal("registration classified launch before Cocoa delivered it")
	}
}

func TestDetectorForwardsNativeLoginClassification(t *testing.T) {
	var detected []bool
	for _, automatic := range []bool{false, true} {
		handle := cgo.NewHandle(func(value bool) { detected = append(detected, value) })
		if automatic {
			ravenLoginLaunchDetected(uintptr(handle), 1)
		} else {
			ravenLoginLaunchDetected(uintptr(handle), 0)
		}
		assertHandleReleased(t, handle)
	}
	if !reflect.DeepEqual(detected, []bool{false, true}) {
		t.Fatalf("unexpected native launch classifications: %v", detected)
	}
}

func TestInvalidNativeCallbackValueStillReleasesItsHandle(t *testing.T) {
	handle := cgo.NewHandle("invalid callback")
	ravenLoginLaunchDetected(uintptr(handle), 0)
	assertHandleReleased(t, handle)
}

func assertHandleReleased(t *testing.T, handle cgo.Handle) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("native callback retained its Go handle")
		}
	}()
	handle.Value()
}
