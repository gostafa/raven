// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package main

import (
	"embed"
)

var (
	//go:embed all:frontend/dist
	assets embed.FS

	//go:embed build/appicon.png
	appIcon []byte
)
