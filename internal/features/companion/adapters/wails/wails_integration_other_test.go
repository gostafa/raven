//go:build !windows && !linux

package wails_test

import (
	"testing"

	companionwails "raven/internal/features/companion/adapters/wails"
)

func TestWindowOptionsNeedNoAdditionalNativeConfiguration(t *testing.T) {
	if err := companionwails.NewWidget(nil).Configure(); err != nil {
		t.Fatal(err)
	}
}
