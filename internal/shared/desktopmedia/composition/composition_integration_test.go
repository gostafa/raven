package composition_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	companionwails "raven/internal/features/companion/adapters/wails"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
	storecomposition "raven/internal/shared/boltstore/composition"
	storeports "raven/internal/shared/boltstore/ports"
	batchcomposition "raven/internal/shared/cffbatch/composition"
	mediaadapter "raven/internal/shared/desktopmedia/adapters/wails"
	"raven/internal/shared/desktopmedia/composition"
	mediaports "raven/internal/shared/desktopmedia/ports"
	schedulingcomposition "raven/internal/shared/scheduling/composition"
)

func TestModuleSharesRegistryAndWiresSavedMedia(t *testing.T) {
	snapshot, err := (configyaml.Loader{}).Load()
	if err != nil {
		t.Fatal(err)
	}
	settings := snapshot.Settings
	var registry *mediaadapter.Registry
	var selection mediaports.Selection
	var routes mediaports.MediaRoutes
	var storeBatch storeports.BatchExecutor
	var mediaBatch mediaports.BatchExecutor
	graph := fxtest.New(
		t,
		composition.Module,
		storecomposition.Module,
		batchcomposition.Module,
		schedulingcomposition.Module,
		fx.Supply(
			fx.Annotate(settings, fx.As(new(configuration.Settings))),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			companionwails.WindowSet{},
		),
		fx.Provide(func() *application.App { return nil }),
		fx.Replace(storecomposition.DatabasePath(filepath.Join(t.TempDir(), "raven.db"))),
		fx.Populate(&registry, &selection, &routes, &storeBatch, &mediaBatch),
	)
	graph.RequireStart()
	t.Cleanup(func() { graph.RequireStop() })
	picker, ok := selection.(*mediaadapter.Picker)
	if !ok || picker.Registry != registry || routes != registry {
		t.Fatal("media contracts do not share the same registry")
	}
	if any(storeBatch) != any(mediaBatch) {
		t.Fatal("batch consumers received different executors")
	}
	if _, err := selection.Select("image"); err == nil {
		t.Fatal("native picker was available without a window")
	}
	handler := routes.Middleware(
		http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) },
		),
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/unrelated", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("fallback status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, settings.MediaRoutePrefix()+"missing", nil),
	)
	if response.Code != http.StatusNotFound {
		t.Fatalf("saved media status = %d", response.Code)
	}
}
