//go:build !darwin

package wails

import "testing"

func TestNonDarwinSoundPlayback(t *testing.T) {
	if err := NewSound(t.Context()).Play(); err != nil {
		t.Fatalf("optional sound playback: %v", err)
	}
}
