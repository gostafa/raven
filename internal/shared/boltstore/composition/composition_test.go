package composition

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/fx/fxtest"

	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
	cffbatch "raven/internal/shared/cffbatch/adapters/cff"
	scheduling "raven/internal/shared/scheduling/adapters/fsrs"
)

func TestStoreLifecycleAndConsumerBindings(t *testing.T) {
	snapshot, err := configyaml.Load()
	if err != nil {
		t.Fatal(err)
	}
	settings := snapshot
	lifecycle := fxtest.NewLifecycle(t)
	path := filepath.Join(t.TempDir(), "raven.db")
	result := newStore(storeParams{
		Path:      DatabasePath(path),
		Lifecycle: lifecycle,
		Batch:     cffbatch.New(), Scheduler: scheduling.New(new(configuration.Library(settings))),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, settings)
	if result.Store != result.Repository || result.Store != result.Reminders ||
		result.Store != result.Settings {
		t.Fatal("consumers do not share storage")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("constructor created storage: %v", err)
	}
	lifecycle.RequireStart()
	if _, err := result.Repository.ListFolders(); err != nil {
		t.Fatal(err)
	}
	lifecycle.RequireStop()
	if _, err := result.Repository.ListFolders(); err == nil {
		t.Fatal("shutdown retained storage")
	}
}

func TestDefaultDatabasePathAndMissingUserDirectory(t *testing.T) {
	path, err := defaultPath()
	if err != nil || path == "" {
		t.Fatalf("default path=%q error=%v", path, err)
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AppData", "")
	if _, err := defaultPath(); err == nil {
		t.Fatal("missing config directory accepted")
	}
}
