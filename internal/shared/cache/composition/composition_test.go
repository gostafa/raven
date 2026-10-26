package composition

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	cacheotter "raven/internal/shared/cache/adapters/otter"
	"raven/internal/shared/cache/ports"
)

func TestReadRetryLifecycle(t *testing.T) {
	lifecycle := fxtest.NewLifecycle(t)
	reads := newReadRetry(lifecycle, slog.New(slog.NewTextHandler(io.Discard, nil)))
	lifecycle.RequireStart()
	if err := reads.Do(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	lifecycle.RequireStop()
	if err := reads.Do(
		t.Context(),
		func(context.Context) error { t.Error("read after shutdown"); return nil },
	); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatal(err)
	}
}

func TestModuleLifecycle(t *testing.T) {
	var chunks ports.Chunks
	graph := fxtest.New(t, Module, fx.Populate(&chunks))
	graph.RequireStart()
	loads := 0
	load := func(context.Context) ([]byte, error) { loads++; return []byte("media"), nil }
	for range 2 {
		if _, err := chunks.Get(t.Context(), "key", load); err != nil {
			t.Fatal(err)
		}
	}
	if loads != 1 {
		t.Fatal("module did not share cache")
	}
	graph.RequireStop()
	if _, err := chunks.Get(t.Context(), "key", load); err != nil || loads != 2 {
		t.Fatal("shutdown retained data", err)
	}
}

func TestCacheStartupFailureRollsBackEarlierCaches(t *testing.T) {
	lifecycle := fxtest.NewLifecycle(t)
	good := Register(lifecycle, &cacheotter.Options[string, string]{MaximumSize: 2})
	Register(lifecycle, &cacheotter.Options[string, string]{MaximumSize: -1})
	if err := lifecycle.Start(t.Context()); err == nil {
		t.Fatal("invalid cache started")
	}
	if err := lifecycle.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	loads := 0
	for range 2 {
		_, err := good.Get(
			t.Context(),
			"key",
			func(context.Context) (string, error) { loads++; return "value", nil },
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if loads != 2 {
		t.Fatal("failed startup retained resources")
	}
}
