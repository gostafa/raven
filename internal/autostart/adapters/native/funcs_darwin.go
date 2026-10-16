//go:build darwin && !ios && !server

package native

/*
#cgo LDFLAGS: -framework Cocoa
void ravenPrepareLoginDetection(void);
*/
import (
	"C"
)

func PrepareLoginDetection(detected func(bool)) {
	nativeLoginDetected = detected
	C.ravenPrepareLoginDetection()
}

//export ravenLoginLaunchDetected
func ravenLoginLaunchDetected(automatic C.int) {
	nativeLoginDetected(automatic != 0)
}
