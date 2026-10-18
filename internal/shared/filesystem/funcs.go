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
	file, err := openAuthorizedSelection(path, os.OpenInRoot)
	if err != nil {
		return nil, fmt.Errorf("open selected media: %w", err)
	}

	return file, nil
}

func openAuthorizedSelection(
	path string,
	openInRoot func(string, string) (*os.File, error),
) (*os.File, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("resolve selected media path: %w", err)
	}

	file, err := openInRoot(filepath.Dir(resolved), filepath.Base(resolved))
	if err != nil {
		return nil, fmt.Errorf("open selected file within root: %w", err)
	}

	return file, nil
}
