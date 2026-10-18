//go:build windows

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package bbolt

import "testing"

func TestWindowsDirectorySyncIsNoop(t *testing.T) {
	if err := syncDirectory(t.TempDir()); err != nil {
		t.Fatalf("sync directory: %v", err)
	}
}
