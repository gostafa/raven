// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package main

import (
	"embed"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte
