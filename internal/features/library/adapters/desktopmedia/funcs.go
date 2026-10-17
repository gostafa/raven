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
	localfiles "raven/internal/shared/filesystem"
)

// NewPicker attaches native selection to an existing HTTP registry.
func NewPicker(
	app *application.App,
	window *application.WebviewWindow,
	registry *Registry,
) *Picker {
	return &Picker{Registry: registry, choose: nativeChooser(app, window)}
}

// NewRegistry creates media routes before native application construction.
func NewRegistry(dependencies *Dependencies, settings *Settings) *Registry {
	saved := dependencies.Saved
	if saved == nil {
		saved = func() http.Handler { return nil }
	}

	return &Registry{
		settings: *settings,
		batch:    dependencies.Batch,
		saved:    saved,
		previews: sync.Map{},
		logger:   dependencies.Logger,
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
	for range paths {
		selected = append(selected, mediaapi.Selection{
			SourcePath: "", OriginalName: "", PreviewURL: "", ByteSize: zero,
		})
	}

	jobs := picker.inspectionJobs(paths, selected, filter)

	err := picker.batch.Run(context.Background(), jobs)
	if err != nil {
		return nil, fmt.Errorf("inspect selection: %w", err)
	}

	picker.registerPreviews(selected)

	return selected, nil
}

func (*Picker) inspectionJobs(
	paths []string,
	selected []mediaapi.Selection,
	filter string,
) []func(context.Context) error {
	jobs := make([]func(context.Context) error, zero, len(paths))
	for index := range paths {
		jobs = append(jobs, func(context.Context) error {
			value, err := describePath(paths[index], filter)
			if err != nil {
				return fmt.Errorf("inspect selected media: %w", err)
			}

			selected[index] = value

			return nil
		})
	}

	return jobs
}

func (picker *Picker) registerPreviews(selected []mediaapi.Selection) {
	for index := range selected {
		key := rand.Text()
		picker.previews.Store(key, selected[index].SourcePath)

		selected[index].PreviewURL = picker.settings.PreviewRoutePrefix + key
	}
}

func describePath(path, filter string) (mediaapi.Selection, error) {
	info, err := os.Stat(path)
	if err != nil {
		return mediaapi.Selection{}, fmt.Errorf("inspect selected file: %w", err)
	}

	err = validateSelection(info, path, filter)
	if err != nil {
		return mediaapi.Selection{}, fmt.Errorf("validate selected file: %w", err)
	}

	return mediaapi.Selection{
		SourcePath:   path,
		OriginalName: filepath.Base(path),
		ByteSize:     info.Size(),
		PreviewURL:   "",
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
func (picker *Registry) Middleware(next http.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasPrefix(request.URL.Path, picker.settings.PreviewRoutePrefix):
			picker.servePreview(writer, request)
		case strings.HasPrefix(request.URL.Path, picker.settings.MediaRoutePrefix):
			picker.serveSaved(writer, request)
		default:
			next.ServeHTTP(writer, request)
		}
	}
}

func (picker *Registry) serveSaved(writer http.ResponseWriter, request *http.Request) {
	handler := picker.saved()
	if handler == nil {
		http.Error(writer, "media storage is unavailable", http.StatusServiceUnavailable)

		return
	}

	handler.ServeHTTP(writer, request)
}

func (picker *Registry) servePreview(writer http.ResponseWriter, request *http.Request) {
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

func (picker *Registry) previewPath(url string) (string, bool) {
	value, exists := picker.previews.Load(
		strings.TrimPrefix(url, picker.settings.PreviewRoutePrefix),
	)
	if !exists {
		return "", false
	}

	path, valid := value.(string)

	return path, valid
}

func (picker *Registry) servePreviewPath(
	writer http.ResponseWriter,
	request *http.Request,
	path string,
) {
	file, err := localfiles.OpenSelected(path)
	if err != nil {
		http.NotFound(writer, request)

		return
	}
	defer picker.closePreview(request.Context(), file)

	servePreviewFile(writer, request, file)
}

func (picker *Registry) closePreview(ctx context.Context, file *os.File) {
	err := file.Close()
	if err != nil {
		picker.logger.WarnContext(
			ctx,
			"could not close media preview",
			//nolint:sloglint // Log field names come from validated embedded configuration.
			slog.Any(picker.settings.LogErrorKey, err),
		)
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
