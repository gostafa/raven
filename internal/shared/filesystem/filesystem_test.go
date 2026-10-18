package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenSelectedReportsRootOpenFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selected.png")
	if err := os.WriteFile(path, []byte("selected"), 0o600); err != nil {
		t.Fatal(err)
	}

	failure := errors.New("rooted open failed")
	var gotRoot, gotPath string
	_, err := openAuthorizedSelection(path, func(root, name string) (*os.File, error) {
		gotRoot, gotPath = root, name
		return nil, failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("unexpected rooted open failure: %v", err)
	}
	resolved, resolveErr := filepath.EvalSymlinks(path)
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	if gotRoot != filepath.Dir(resolved) || gotPath != filepath.Base(resolved) {
		t.Fatalf("opened outside selected file root: %q, %q", gotRoot, gotPath)
	}
}
