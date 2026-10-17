//go:build !darwin || ios || server

package wails

// PrepareLoginDetection registers the native launch classification callback.
func (Detector) PrepareLoginDetection(detected func(bool)) {
	detected(false)
}
