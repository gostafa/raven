// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
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
	"raven/internal/shared/desktopmedia/ports"
	localfiles "raven/internal/shared/filesystem"
)

// NewPicker attaches native selection to an existing HTTP preview registry.
func NewPicker(
	app *application.App,
	window *application.WebviewWindow,
	registry *Registry,
) *Picker {
	return &Picker{
		choose: nativeChooser(
			app,
			window,
			(*application.OpenFileDialogStruct).PromptForMultipleSelection,
		),
		inspect: registry.inspect,
	}
}

// NewRegistry binds media routes, inspection, and preview storage to injected dependencies.
func NewRegistry(
	batch ports.BatchExecutor,
	dependencies *Dependencies,
	settings *Settings,
) *Registry {
	return bindRegistry(newMediaState(batch, dependencies, settings))
}

func newMediaState(
	batch ports.BatchExecutor,
	dependencies *Dependencies,
	settings *Settings,
) *mediaState {
	saved := dependencies.Saved
	if saved == nil {
		saved = func() http.Handler { return nil }
	}

	return &mediaState{
		runBatch:           batch.Run,
		saved:              saved,
		logger:             dependencies.Logger,
		mediaRoutePrefix:   settings.MediaRoutePrefix,
		previewRoutePrefix: settings.PreviewRoutePrefix,
		logErrorKey:        settings.LogErrorKey,
		previews:           sync.Map{},
	}
}

func bindRegistry(state *mediaState) *Registry {
	return &Registry{
		middleware: func(next http.Handler) http.HandlerFunc { return middleware(state, next) },
		inspect: func(paths []string, filter string) (json.RawMessage, error) {
			return inspectMedia(state, paths, filter)
		},
	}
}

func inspectMedia(state *mediaState, paths []string, filter string) (json.RawMessage, error) {
	selected, err := describePaths(state, paths, filter)
	if err != nil {
		return nil, fmt.Errorf("inspect media: %w", err)
	}

	encoded, err := marshalSelection(selected)

	return encoded, errors.Join(err)
}

func nativeChooser(
	app *application.App,
	window *application.WebviewWindow,
	prompt func(*application.OpenFileDialogStruct) ([]string, error),
) func(string, string) ([]string, error) {
	return func(kind, filter string) ([]string, error) {
		if app == nil || window == nil {
			return nil, errFileSelectionIsUnavailable
		}

		dialog := app.Dialog.OpenFile().AttachToWindow(window).CanChooseFiles(true).
			CanChooseDirectories(false).
			AllowsOtherFileTypes(false).AddFilter(kind+" files", filter)

		paths, err := prompt(dialog)
		if err != nil {
			return nil, fmt.Errorf("select native media files: %w", err)
		}

		return paths, nil
	}
}

// Select selects supported media and returns its temporary preview capabilities.
func (picker *Picker) Select(kind string) (string, error) {
	selected, err := selectMedia(picker.choose, picker.inspect, kind)

	return string(selected), errors.Join(err)
}

func selectMedia(
	choose func(string, string) ([]string, error),
	inspect func([]string, string) (json.RawMessage, error),
	kind string,
) (json.RawMessage, error) {
	filter := selectionFilter(kind)
	if filter == "" {
		return nil, errInvalidMediaType
	}

	paths, err := choose(kind, filter)
	if err != nil {
		return nil, fmt.Errorf("select media: %w", err)
	}

	encoded, err := inspect(paths, filter)

	return encoded, errors.Join(err)
}

func selectionFilter(kind string) string {
	return map[string]string{
		"image": "*.jpg;*.jpeg;*.png;*.gif;*.webp;*.avif",
		"audio": "*.mp3;*.wav;*.m4a;*.ogg;*.oga;*.flac",
		"video": "*.mp4;*.m4v;*.webm;*.ogv;*.mov",
	}[kind]
}

func describePaths(picker *mediaState,
	paths []string,
	filter string,
) ([]mediaapi.Selection, error) {
	selected := make([]mediaapi.Selection, zero, len(paths))
	for range paths {
		selected = append(selected, mediaapi.Selection{
			SourcePath: "", OriginalName: "", PreviewURL: "", ByteSize: zero,
		})
	}

	jobs := inspectionJobs(paths, selected, filter)

	err := picker.runBatch(context.Background(), jobs)
	if err != nil {
		return nil, fmt.Errorf("inspect selection: %w", err)
	}

	registerPreviews(picker, selected)

	return selected, nil
}

func inspectionJobs(
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

func registerPreviews(picker *mediaState, selected []mediaapi.Selection) {
	for index := range selected {
		key := rand.Text()
		picker.previews.Store(key, selected[index].SourcePath)

		selected[index].PreviewURL = picker.previewRoutePrefix + key
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
// Middleware serves selected previews and saved media before delegating other routes.
func (registry *Registry) Middleware(next http.Handler) http.HandlerFunc {
	return registry.middleware(next)
}

func middleware(picker *mediaState, next http.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasPrefix(request.URL.Path, picker.previewRoutePrefix):
			servePreview(
				request.Context(),
				picker,
				&previewRequest{writer: writer, request: request, path: ""},
			)
		case strings.HasPrefix(request.URL.Path, picker.mediaRoutePrefix):
			serveSaved(picker, writer, request)
		default:
			next.ServeHTTP(writer, request)
		}
	}
}

func serveSaved(picker *mediaState, writer http.ResponseWriter, request *http.Request) {
	handler := picker.saved()
	if handler == nil {
		http.Error(writer, "media storage is unavailable", http.StatusServiceUnavailable)

		return
	}

	handler.ServeHTTP(writer, request)
}

func servePreview(ctx context.Context, picker *mediaState, preview *previewRequest) {
	writer, request := preview.writer, preview.request
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.WriteHeader(http.StatusMethodNotAllowed)

		return
	}

	path, ok := previewPath(picker, request.URL.Path)
	if !ok {
		http.NotFound(writer, request)

		return
	}

	preview.path = path
	servePreviewPath(ctx, picker, preview)
}

func previewPath(picker *mediaState, url string) (string, bool) {
	value, exists := picker.previews.Load(
		strings.TrimPrefix(url, picker.previewRoutePrefix),
	)
	if !exists {
		return "", false
	}

	path, valid := value.(string)

	return path, valid
}

func servePreviewPath(ctx context.Context, picker *mediaState, request *previewRequest) {
	file, err := localfiles.OpenSelected(request.path)
	if err != nil {
		http.NotFound(request.writer, request.request)

		return
	}
	defer closePreview(ctx, picker, file)

	servePreviewFile(request.writer, request.request, file)
}

func closePreview(ctx context.Context, picker *mediaState, file *os.File) {
	err := file.Close()
	if err != nil {
		picker.logger.WarnContext(
			ctx,
			"could not close media preview",
			//nolint:sloglint // Log field names come from validated embedded configuration.
			slog.Any(picker.logErrorKey, err),
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

func marshalSelection(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode media value: %w", err)
	}

	return encoded, nil
}
