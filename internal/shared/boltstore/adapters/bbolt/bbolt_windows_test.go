//go:build windows

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package bbolt

import (
	"path/filepath"
	"testing"
)

func TestWindowsDirectorySyncIsNoop(t *testing.T) {
	if err := syncDirectory(t.TempDir()); err != nil {
		t.Fatalf("sync directory: %v", err)
	}
}

func TestWindowsDirectorySyncIgnoresMissingDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "missing")
	if err := syncDirectory(directory); err != nil {
		t.Fatalf("sync missing directory: %v", err)
	}
}
