//go:build !windows && !linux

package native_test

import (
	"testing"

	"raven/internal/companion/adapters/native"
)

func TestWindowOptionsNeedNoAdditionalNativeConfiguration(t *testing.T) {
	if err := native.Configure(nil); err != nil {
		t.Fatal(err)
	}
}
