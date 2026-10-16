//go:build !darwin || ios || server

package native

func PrepareLoginDetection(detected func(bool)) {
	detected(false)
}
