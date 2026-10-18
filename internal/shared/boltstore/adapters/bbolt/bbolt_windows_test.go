//go:build windows

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package bbolt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
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

func TestWindowsExistingDatabaseFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")

	exists, err := existingDatabaseFile(path)
	if err != nil || exists {
		t.Fatalf("missing database: exists=%v, error=%v", exists, err)
	}
	if err := validateExistingDatabase(path); err != nil {
		t.Fatalf("validate missing database: %v", err)
	}

	if err := os.WriteFile(path, nil, privateFileMode); err != nil {
		t.Fatal(err)
	}
	exists, err = existingDatabaseFile(path)
	if err != nil || exists {
		t.Fatalf("empty database: exists=%v, error=%v", exists, err)
	}
	if err := validateExistingDatabase(path); err != nil {
		t.Fatalf("validate empty database: %v", err)
	}
}

func TestWindowsValidateExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	database, err := bolt.Open(path, privateFileMode, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	exists, err := existingDatabaseFile(path)
	if err != nil || !exists {
		t.Fatalf("valid database: exists=%v, error=%v", exists, err)
	}
	if err := validateExistingDatabase(path); err != nil {
		t.Fatalf("validate valid database: %v", err)
	}

	if err := os.WriteFile(path, []byte("not a database"), privateFileMode); err != nil {
		t.Fatal(err)
	}
	exists, err = existingDatabaseFile(path)
	if err != nil || !exists {
		t.Fatalf("invalid database: exists=%v, error=%v", exists, err)
	}
	if err := validateExistingDatabase(path); err == nil ||
		!strings.Contains(err.Error(), "validate existing database") {
		t.Fatalf("expected wrapped validation error, got %v", err)
	}
}

func TestWindowsDatabasePathInspectionErrors(t *testing.T) {
	path := string([]byte{0})
	if _, err := existingDatabaseFile(path); err == nil ||
		!strings.Contains(err.Error(), "inspect database file") {
		t.Fatalf("expected wrapped file inspection error, got %v", err)
	}
	if err := validateExistingDatabase(path); err == nil ||
		!strings.Contains(err.Error(), "inspect existing database") ||
		!strings.Contains(err.Error(), "inspect database file") {
		t.Fatalf("expected wrapped inspection error, got %v", err)
	}
}
