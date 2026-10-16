//go:build !darwin || ios || server

package native

// PrepareLoginDetection registers the native launch classification callback.
func (Detector) PrepareLoginDetection(detected func(bool)) {
	detected(false)
}
