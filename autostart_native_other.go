//go:build !darwin || ios || server

package main

func prepareNativeLoginDetection(detected func(bool)) {
	detected(false)
}
