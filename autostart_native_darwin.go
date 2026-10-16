//go:build darwin && !ios && !server

package main

/*
#cgo LDFLAGS: -framework Cocoa
void ravenPrepareLoginDetection(void);
*/
import "C"

// Set once on the main goroutine, before Cocoa's launch notifications.
var nativeLoginDetected func(bool)

func prepareNativeLoginDetection(detected func(bool)) {
	nativeLoginDetected = detected
	C.ravenPrepareLoginDetection()
}

//export ravenLoginLaunchDetected
func ravenLoginLaunchDetected(automatic C.int) {
	nativeLoginDetected(automatic != 0)
}
