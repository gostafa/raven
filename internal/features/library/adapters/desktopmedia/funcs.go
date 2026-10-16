// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package desktopmedia

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	mediaapi "raven/api/media"
	"raven/internal/shared/filesystem"
)

// New creates a native picker and an isolated preview registry.
func New(
	app *application.App,
	window *application.WebviewWindow,
	saved func() http.Handler,
) *Picker {
	if saved == nil {
		saved = func() http.Handler { return nil }
	}

	return &Picker{
		choose:   nativeChooser(app, window),
		saved:    saved,
		previews: sync.Map{},
		logger:   slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
}

func nativeChooser(
	app *application.App,
	window *application.WebviewWindow,
) func(string, string) ([]string, error) {
	return func(kind, filter string) ([]string, error) {
		if app == nil || window == nil {
			return nil, errFileSelectionIsUnavailable
		}

		paths, err := app.Dialog.OpenFile().AttachToWindow(window).CanChooseFiles(true).
			CanChooseDirectories(false).
			AllowsOtherFileTypes(false).AddFilter(kind+" files", filter).
			PromptForMultipleSelection()
		if err != nil {
			return nil, fmt.Errorf("select native media files: %w", err)
		}

		return paths, nil
	}
}

// Select selects supported media files and issues temporary preview URLs.
func (picker *Picker) Select(kind string) ([]mediaapi.Selection, error) {
	filter := selectionFilter(kind)
	if filter == "" {
		return nil, errInvalidMediaType
	}

	paths, err := picker.choose(kind, filter)
	if err != nil {
		return nil, fmt.Errorf("select media: %w", err)
	}

	result, forwardErr := picker.describePaths(paths, filter)
	if forwardErr != nil {
		return result, fmt.Errorf("select: %w", forwardErr)
	}

	return result, nil
}

func selectionFilter(kind string) string {
	return map[string]string{
		"image": "*.jpg;*.jpeg;*.png;*.gif;*.webp;*.avif",
		"audio": "*.mp3;*.wav;*.m4a;*.ogg;*.oga;*.flac",
		"video": "*.mp4;*.m4v;*.webm;*.ogv;*.mov",
	}[kind]
}

func (picker *Picker) describePaths(
	paths []string,
	filter string,
) ([]mediaapi.Selection, error) {
	selected := make([]mediaapi.Selection, zero, len(paths))
	for index := range paths {
		value, err := picker.describePath(paths[index], filter)
		if err != nil {
			return nil, fmt.Errorf("inspect selected media: %w", err)
		}

		selected = append(selected, value)
	}

	return selected, nil
}

func (picker *Picker) describePath(path, filter string) (mediaapi.Selection, error) {
	info, err := os.Stat(path)
	if err != nil {
		return mediaapi.Selection{}, fmt.Errorf("inspect selected file: %w", err)
	}

	err = validateSelection(info, path, filter)
	if err != nil {
		return mediaapi.Selection{}, fmt.Errorf("validate selected file: %w", err)
	}

	key := rand.Text()
	picker.previews.Store(key, path)

	return mediaapi.Selection{
		SourcePath:   path,
		OriginalName: filepath.Base(path),
		ByteSize:     info.Size(),
		PreviewURL:   previewRoutePrefix + key,
	}, nil
}

func validateSelection(info os.FileInfo, path, filter string) error {
	if !info.Mode().IsRegular() || info.Size() == zero {
		return errSelectANonemptyMediaFile
	}

	extension := strings.ToLower(filepath.Ext(path))
	if !strings.Contains(";"+filter+";", ";*"+extension+";") {
		return errChooseASupportedFileForThisMedia
	}

	return nil
}

// Middleware serves selected previews and saved media before delegating other routes.
func (picker *Picker) Middleware(next http.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasPrefix(request.URL.Path, previewRoutePrefix):
			picker.servePreview(writer, request)
		case strings.HasPrefix(request.URL.Path, mediaRoutePrefix):
			picker.serveSaved(writer, request)
		default:
			next.ServeHTTP(writer, request)
		}
	}
}

func (picker *Picker) serveSaved(writer http.ResponseWriter, request *http.Request) {
	handler := picker.saved()
	if handler == nil {
		http.Error(writer, "media storage is unavailable", http.StatusServiceUnavailable)

		return
	}

	handler.ServeHTTP(writer, request)
}

func (picker *Picker) servePreview(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.WriteHeader(http.StatusMethodNotAllowed)

		return
	}

	path, ok := picker.previewPath(request.URL.Path)
	if !ok {
		http.NotFound(writer, request)

		return
	}

	picker.servePreviewPath(writer, request, path)
}

func (picker *Picker) previewPath(url string) (string, bool) {
	value, exists := picker.previews.Load(strings.TrimPrefix(url, previewRoutePrefix))
	if !exists {
		return "", false
	}

	path, valid := value.(string)

	return path, valid
}

func (picker *Picker) servePreviewPath(
	writer http.ResponseWriter,
	request *http.Request,
	path string,
) {
	file, err := filesystem.OpenSelected(path)
	if err != nil {
		http.NotFound(writer, request)

		return
	}
	defer picker.closePreview(request.Context(), file)

	servePreviewFile(writer, request, file)
}

func (picker *Picker) closePreview(ctx context.Context, file *os.File) {
	err := file.Close()
	if err != nil {
		picker.logger.WarnContext(ctx, "could not close media preview", slog.Any(logErrorKey, err))
	}
}

func servePreviewFile(
	writer http.ResponseWriter,
	request *http.Request,
	file *os.File,
) {
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(writer, request)

		return
	}

	setPreviewHeaders(writer, info.Name())
	http.ServeContent(writer, request, info.Name(), info.ModTime(), file)
}

func setPreviewHeaders(writer http.ResponseWriter, name string) {
	writer.Header().Set("Content-Type", mime.TypeByExtension(strings.ToLower(filepath.Ext(name))))
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Cache-Control", "no-store")
}
