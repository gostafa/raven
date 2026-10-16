// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package desktopmedia

import (
	"net/http"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type picker struct {
	app        *application.App
	mainWindow *application.WebviewWindow
	saved      func() http.Handler
	previews   sync.Map
}
