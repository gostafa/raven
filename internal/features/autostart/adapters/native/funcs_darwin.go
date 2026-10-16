//go:build darwin && !ios && !server

package native

import (
	/*
	   #cgo LDFLAGS: -framework Cocoa
	   #include <stdint.h>
	   void ravenPrepareLoginDetection(uintptr_t handle);
	*/
	"C"
	"runtime/cgo"
)

// PrepareLoginDetection registers the native launch classification callback.
func (Detector) PrepareLoginDetection(detected func(bool)) {
	handle := cgo.NewHandle(detected)
	C.ravenPrepareLoginDetection(C.uintptr_t(handle))
}

//export ravenLoginLaunchDetected
func ravenLoginLaunchDetected(handle uintptr, automatic C.int) {
	dispatchLoginDetection(cgo.Handle(handle), automatic != 0)
}

func dispatchLoginDetection(handle cgo.Handle, automatic bool) {
	defer handle.Delete()

	detected, ok := handle.Value().(func(bool))
	if ok {
		detected(automatic)
	}
}
