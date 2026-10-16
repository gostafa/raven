//go:build !windows

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package boltstore

import (
	"errors"
	"fmt"
	"os"
)

func syncDirectory(path string) error {
	directory, err := os.OpenInRoot(path, ".")
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}

	err = errors.Join(directory.Sync(), directory.Close())
	if err != nil {
		return fmt.Errorf("sync and close directory: %w", err)
	}

	return nil
}
