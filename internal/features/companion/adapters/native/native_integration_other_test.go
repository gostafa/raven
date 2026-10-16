//go:build !windows && !linux

package native_test

import (
	"testing"

	"raven/internal/features/companion/adapters/native"
)

func TestWindowOptionsNeedNoAdditionalNativeConfiguration(t *testing.T) {
	if err := native.New(nil).Configure(); err != nil {
		t.Fatal(err)
	}
}
