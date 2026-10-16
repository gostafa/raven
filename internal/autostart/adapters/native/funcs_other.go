//go:build !darwin || ios || server

package native

// PrepareLoginDetection registers the native launch classification callback.
func PrepareLoginDetection(detected func(bool)) {
	detected(false)
}
