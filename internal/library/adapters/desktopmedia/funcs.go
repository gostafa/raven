// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package desktopmedia

import (
	"crypto/rand"
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"raven/internal/library"
)

func New(
	app *application.App,
	window *application.WebviewWindow,
	saved func() http.Handler,
) *picker {
	if saved == nil {
		saved = func() http.Handler { return nil }
	}

	return &picker{app: app, mainWindow: window, saved: saved}
}

func (r *picker) Select(kind string) ([]library.SelectedMedia, error) {
	filters := map[string]string{
		"image": "*.jpg;*.jpeg;*.png;*.gif;*.webp;*.avif",
		"audio": "*.mp3;*.wav;*.m4a;*.ogg;*.oga;*.flac",
		"video": "*.mp4;*.m4v;*.webm;*.ogv;*.mov",
	}
	filter, ok := filters[kind]

	if !ok {
		return nil, errors.New("invalid media type")
	}

	if r.app == nil || r.mainWindow == nil {
		return nil, errors.New("file selection is unavailable")
	}

	paths, err := r.app.Dialog.OpenFile().
		AttachToWindow(r.mainWindow).
		CanChooseFiles(true).
		CanChooseDirectories(false).
		AllowsOtherFileTypes(false).
		AddFilter(kind+" files", filter).
		PromptForMultipleSelection()
	if err != nil {
		return nil, err
	}

	selected := make([]library.SelectedMedia, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}

		if !info.Mode().IsRegular() || info.Size() == 0 {
			return nil, errors.New("select a nonempty media file")
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !strings.Contains(";"+filter+";", ";*"+ext+";") {
			return nil, errors.New("choose a supported file for this media type")
		}

		token := rand.Text()
		r.previews.Store(token, path)

		selected = append(
			selected,
			library.SelectedMedia{
				SourcePath:   path,
				OriginalName: filepath.Base(path),
				ByteSize:     info.Size(),
				PreviewURL:   "/media-preview/" + token,
			},
		)
	}

	return selected, nil
}

func (r *picker) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/media-preview/") {
			if req.Method != http.MethodGet && req.Method != http.MethodHead {
				w.WriteHeader(http.StatusMethodNotAllowed)

				return
			}

			path, ok := r.previews.Load(strings.TrimPrefix(req.URL.Path, "/media-preview/"))
			if !ok {
				http.NotFound(w, req)

				return
			}

			file, err := os.Open(path.(string))
			if err != nil {
				http.NotFound(w, req)

				return
			}
			defer file.Close()

			info, err := file.Stat()
			if err != nil || !info.Mode().IsRegular() {
				http.NotFound(w, req)

				return
			}

			w.Header().
				Set("Content-Type", mime.TypeByExtension(strings.ToLower(filepath.Ext(path.(string)))))
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "no-store")
			http.ServeContent(w, req, info.Name(), info.ModTime(), file)

			return
		}

		if !strings.HasPrefix(req.URL.Path, "/media/") {
			next.ServeHTTP(w, req)

			return
		}

		handler := r.saved()
		if handler == nil {
			http.Error(w, "media storage is unavailable", http.StatusServiceUnavailable)

			return
		}

		handler.ServeHTTP(w, req)
	})
}
