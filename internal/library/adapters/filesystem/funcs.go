// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
)

// OpenSelected resolves an authorized selection and confines the open to its directory.
// A selection may itself be a symlink to a file in another directory.
func OpenSelected(path string) (*os.File, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("resolve selected media path: %w", err)
	}

	file, err := os.OpenInRoot(filepath.Dir(resolved), filepath.Base(resolved))
	if err != nil {
		return nil, fmt.Errorf("open selected media: %w", err)
	}

	return file, nil
}
