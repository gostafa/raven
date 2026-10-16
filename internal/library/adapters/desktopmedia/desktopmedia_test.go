package desktopmedia

import (
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
