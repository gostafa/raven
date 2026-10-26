//go:build !darwin && !linux && !windows

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"os"
)

func fileIdentity(info os.FileInfo) string {
	return info.Name()
}
