//go:build darwin && !ios && !server

package native

// Set before Cocoa launch notifications.
var nativeLoginDetected func(bool)
