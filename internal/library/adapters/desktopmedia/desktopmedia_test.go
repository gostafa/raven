package desktopmedia

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSelectedMediaPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := New(nil, nil, nil)
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
	service := New(nil, nil, nil)
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

func TestPreviewRegistryAndRoutingFailures(t *testing.T) {
	service := New(nil, nil, func() http.Handler {
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
		t.Fatal(err)
	}
	service := New(nil, nil, nil)
	service.previews.Store("selected", link)
	response := httptest.NewRecorder()
	service.Middleware(http.NotFoundHandler()).
		ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/media-preview/selected", nil))
	if response.Code != http.StatusOK || response.Body.String() != "selected" {
		t.Fatalf("symlink preview: %d %s", response.Code, response.Body.String())
	}
}
