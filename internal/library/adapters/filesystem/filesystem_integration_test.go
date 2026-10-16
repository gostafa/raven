package filesystem_test

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"raven/internal/library/adapters/filesystem"
)

func TestSelectedFilesResolveSymlinksAndReportAccessFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selected.png")
	if err := os.WriteFile(path, []byte("selected"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "selection.png")
	if err := os.Symlink(path, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink permission is unavailable")
		}
		t.Fatal(err)
	}
	file, err := filesystem.OpenSelected(link)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := io.ReadAll(file)
	if err != nil || string(contents) != "selected" {
		t.Fatalf("selection: %s %v", contents, err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := filesystem.OpenSelected(path + ".missing"); err == nil {
		t.Fatal("accepted missing file")
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Error(err)
		}
	})
	if file, err := filesystem.OpenSelected(path); err == nil {
		file.Close()
		t.Fatal("opened unreadable selection")
	}
}
