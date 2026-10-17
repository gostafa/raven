package wails

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	mediaapi "raven/api/media"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
	cffbatch "raven/internal/shared/cffbatch/adapters/cff"
)

func TestNativeChooserConfiguresDialogAndPreservesResult(t *testing.T) {
	app := application.New(application.Options{})
	window := &application.WebviewWindow{}
	failure := errors.New("native dialog failed")
	for _, nativeFailure := range []error{nil, failure} {
		choose := nativeChooser(
			app,
			window,
			func(dialog *application.OpenFileDialogStruct) ([]string, error) {
				if dialog == nil {
					t.Fatal("missing dialog")
				}
				return []string{"selected.png"}, nativeFailure
			},
		)
		paths, err := choose("image", "*.png")
		if nativeFailure != nil {
			if !errors.Is(err, nativeFailure) {
				t.Fatalf("native failure: %v", err)
			}
		} else if err != nil || len(paths) != 1 || paths[0] != "selected.png" {
			t.Fatalf("selection=%v error=%v", paths, err)
		}
	}
}

func TestUnavailablePickerAndSavedMediaReturnAvailabilityErrors(t *testing.T) {
	picker := testPicker(nil)
	if _, err := picker.Select("image"); !errors.Is(err, errFileSelectionIsUnavailable) {
		t.Fatalf("missing native window: %v", err)
	}
	response := httptest.NewRecorder()
	picker.Middleware(http.NotFoundHandler()).
		ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/media/folder/card/item", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing media store returned %d", response.Code)
	}
}

func TestClosedPreviewFilesAreRejectedAndCleanupFailuresAreLogged(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "preview-*.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	picker := testPicker(nil)
	picker.logger = slog.New(slog.NewTextHandler(&output, nil))
	picker.closePreview(t.Context(), file)
	if !bytes.Contains(output.Bytes(), []byte("could not close media preview")) {
		t.Fatal("preview cleanup failure was not logged")
	}
	response := httptest.NewRecorder()
	servePreviewFile(
		response,
		httptest.NewRequest(http.MethodGet, "/media-preview/closed", nil),
		file,
	)
	if response.Code != http.StatusNotFound {
		t.Fatalf("closed preview file returned %d", response.Code)
	}
}

func TestSelectedMediaPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := testPicker(nil)
	service.previews.Store("selected-token", path)
	handler := service.Middleware(http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodGet, "/media-preview/selected-token", nil)
	request.Header.Set("Range", "bytes=2-5")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusPartialContent || response.Body.String() != "2345" ||
		response.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf(
			"preview range: %d %s %v",
			response.Code,
			response.Body.String(),
			response.Header(),
		)
	}
	for _, url := range []string{"/media-preview/unknown", "/media-preview/" + path} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, url, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("unselected path served: %s", url)
		}
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/media-preview/selected-token", nil),
	)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status: %d", response.Code)
	}
}

func TestSelectionValidatesFilesAndCreatesPreviews(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "IMAGE.PNG")
	if err := os.WriteFile(path, []byte("selected"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := testPicker(nil)
	service.choose = func(kind, filter string) ([]string, error) {
		if kind != "image" || filter == "" {
			t.Fatal("missing native filter")
		}
		return []string{path}, nil
	}
	selected, err := service.Select("image")
	if err != nil || len(selected) != 1 || selected[0].SourcePath != path ||
		selected[0].ByteSize != 8 ||
		selected[0].OriginalName != "IMAGE.PNG" {
		t.Fatalf("selection: %+v %v", selected, err)
	}
	response := httptest.NewRecorder()
	service.Middleware(http.NotFoundHandler()).
		ServeHTTP(response, httptest.NewRequest(http.MethodHead, selected[0].PreviewURL, nil))
	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("HEAD preview: %d", response.Code)
	}
	for _, invalid := range []string{directory, filepath.Join(directory, "missing.png"), filepath.Join(directory, "empty.png"), filepath.Join(directory, "file.html")} {
		if filepath.Ext(invalid) == ".html" {
			if err := os.WriteFile(invalid, []byte("active"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if filepath.Base(invalid) == "empty.png" {
			if err := os.WriteFile(invalid, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		service.choose = func(string, string) ([]string, error) { return []string{invalid}, nil }
		if _, err := service.Select("image"); err == nil {
			t.Fatalf("accepted %s", invalid)
		}
	}
	service.choose = func(string, string) ([]string, error) { return nil, errors.New("dialog failed") }
	if _, err := service.Select("audio"); err == nil {
		t.Fatal("lost dialog error")
	}
	service.choose = func(string, string) ([]string, error) { return nil, nil }
	if values, err := service.Select("video"); err != nil || len(values) != 0 {
		t.Fatalf("cancel: %+v %v", values, err)
	}
	if _, err := service.Select("invalid"); err == nil {
		t.Fatal("accepted invalid media kind")
	}
}

func TestBatchSelectionPreservesOrderAndRegistersOnlySuccessfulBatches(t *testing.T) {
	directory := t.TempDir()
	paths := make([]string, 8)
	for index := range paths {
		paths[index] = filepath.Join(directory, strings.Repeat("a", index+1)+".png")
		if err := os.WriteFile(paths[index], []byte("selected"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	picker := testPicker(nil)
	selected, err := picker.describePaths(paths, selectionFilter("image"))
	if err != nil {
		t.Fatal(err)
	}
	for index := range paths {
		path, exists := picker.previewPath(selected[index].PreviewURL)
		if !exists || path != paths[index] || selected[index].SourcePath != paths[index] {
			t.Fatalf("selection %d: %+v preview=%s", index, selected[index], path)
		}
	}
	failed := testPicker(nil)
	paths[1] = filepath.Join(directory, "missing.png")
	if values, err := failed.describePaths(
		paths,
		selectionFilter("image"),
	); err == nil ||
		values != nil {
		t.Fatalf("failed selection: %+v %v", values, err)
	}
	failed.previews.Range(func(key, value any) bool {
		t.Errorf("failed batch registered preview %v: %v", key, value)
		return true
	})
}

func TestPreviewRegistryAndRoutingFailures(t *testing.T) {
	service := testPicker(func() http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) },
		)
	})
	service.previews.Store("wrong-type", 1)
	service.previews.Store("gone", filepath.Join(t.TempDir(), "missing.png"))
	service.previews.Store("directory", t.TempDir())
	handler := service.Middleware(
		http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) },
		),
	)
	for _, scenario := range []struct {
		path   string
		status int
	}{
		{"/media-preview/wrong-type", http.StatusNotFound},
		{"/media-preview/gone", http.StatusNotFound},
		{"/media-preview/directory", http.StatusNotFound},
		{"/media/folder/card/item", http.StatusAccepted},
		{"/", http.StatusNoContent},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, scenario.path, nil))
		if response.Code != scenario.status {
			t.Fatalf("%s: %d", scenario.path, response.Code)
		}
	}
}

func TestSelectedSymlinkMayReferToAnotherDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.png")
	if err := os.WriteFile(path, []byte("selected"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link.png")
	if err := os.Symlink(path, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink permission is unavailable")
		}
		t.Fatal(err)
	}
	service := testPicker(nil)
	service.previews.Store("selected", link)
	response := httptest.NewRecorder()
	service.Middleware(http.NotFoundHandler()).
		ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/media-preview/selected", nil))
	if response.Code != http.StatusOK || response.Body.String() != "selected" {
		t.Fatalf("symlink preview: %d %s", response.Code, response.Body.String())
	}
}

func configurationFixture() configuration.Settings {
	settings, err := configyaml.Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func mediaSettingsFixture() *Settings {
	settings := configurationFixture()
	return &Settings{
		MediaRoutePrefix:   settings.MediaRoutePrefix(),
		PreviewRoutePrefix: settings.PreviewRoutePrefix(),
		LogErrorKey:        settings.LogErrorKey(),
	}
}

func testPicker(saved func() http.Handler) *testMedia {
	state := newMediaState(
		cffbatch.New(),
		saved,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		mediaSettingsFixture(),
	)
	registry := bindRegistry(state)
	return &testMedia{Picker: NewPicker(nil, nil, registry), Registry: registry, mediaState: state}
}

func TestConfiguredMediaRoutesReachPickerAndMiddleware(t *testing.T) {
	settings := mediaSettingsFixture()
	settings.MediaRoutePrefix = "/saved/"
	settings.PreviewRoutePrefix = "/preview/"
	registry := NewRegistry(cffbatch.New(), func() http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) },
		)
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), settings)
	picker := NewPicker(nil, nil, registry)
	path := filepath.Join(t.TempDir(), "selected.png")
	if err := os.WriteFile(path, []byte("selected image"), 0o600); err != nil {
		t.Fatal(err)
	}
	picker.choose = func(string, string) ([]string, error) { return []string{path}, nil }
	selected, err := picker.Select("image")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(selected[0].PreviewURL, "/preview/") {
		t.Fatal(selected)
	}
	handler := registry.Middleware(
		http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) },
		),
	)
	for route, status := range map[string]int{
		"/saved/folder/card/item": http.StatusAccepted,
		selected[0].PreviewURL:    http.StatusOK,
		"/media/folder/card/item": http.StatusNoContent,
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
		if response.Code != status {
			t.Fatalf("%s: got %d, want %d", route, response.Code, status)
		}
	}
}

// testMedia exposes implementation seams only to implementation tests.
type testMedia struct {
	*Picker
	*Registry
	*mediaState
}

func (media *testMedia) describePaths(paths []string, filter string) ([]mediaapi.Selection, error) {
	return describePaths(media.mediaState, paths, filter)
}

func (media *testMedia) previewPath(path string) (string, bool) {
	return previewPath(media.mediaState, path)
}

func (media *testMedia) closePreview(ctx context.Context, file *os.File) {
	closePreview(media.mediaState, ctx, file)
}
