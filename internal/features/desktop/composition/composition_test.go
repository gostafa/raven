package composition

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"raven/internal/features/autostart/adapters/launch"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
	desktopwails "raven/internal/features/desktop/adapters/wails"
	"raven/internal/shared/lifetime"
)

func TestBridgeLifetimeDeadlinesAndRepeatedShutdown(t *testing.T) {
	runtime := lifetime.New()
	starts, stops := 0, 0
	graph := fxtest.New(t, fx.Invoke(func(lc fx.Lifecycle) {
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				starts++
				if _, ok := ctx.Deadline(); !ok {
					t.Error("startup has no deadline")
				}
				return nil
			},
			OnStop: func(ctx context.Context) error {
				stops++
				if ctx.Err() != nil {
					t.Error("shutdown inherited canceled startup context")
				}
				return nil
			},
		})
	}))
	bridge := NewBridge(graph, runtime)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	for range 2 {
		if err := bridge.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.Context().Err() != nil {
		t.Fatal("startup deadline canceled the runtime")
	}
	cancel()
	select {
	case <-runtime.Context().Done():
	case <-t.Context().Done():
		t.Fatal("runtime did not inherit service cancellation")
	}
	for range 2 {
		if err := bridge.ServiceShutdown(); err != nil {
			t.Fatal(err)
		}
	}
	if starts != 1 || stops != 1 {
		t.Fatalf("starts=%d stops=%d", starts, stops)
	}
}

func TestBridgeIsolatesStartupAndShutdownFailures(t *testing.T) {
	startFailure, stopFailure := errors.New("startup failure"), errors.New("shutdown failure")
	for _, failStart := range []bool{false, true} {
		t.Run(
			fmt.Sprintf("startup_failure_%t", failStart),
			func(t *testing.T) {
				runtime := lifetime.New()
				graph := fxtest.New(t, fx.Invoke(func(lc fx.Lifecycle) {
					lc.Append(fx.Hook{OnStart: func(context.Context) error {
						if failStart {
							return startFailure
						}
						return nil
					}, OnStop: func(context.Context) error { return stopFailure }})
				}))
				bridge := NewBridge(graph, runtime)
				err := bridge.ServiceStartup(t.Context(), application.ServiceOptions{})
				if failStart {
					if err == nil || errors.Is(err, startFailure) || errors.Unwrap(err) != nil {
						t.Fatalf("startup error leaked: %v", err)
					}
					if runtime.Context().Err() == nil {
						t.Fatal("failed startup retained runtime")
					}
				} else if err != nil {
					t.Fatal(err)
				}
				err = bridge.ServiceShutdown()
				if !failStart &&
					(err == nil || errors.Is(err, stopFailure) || errors.Unwrap(err) != nil) {
					t.Fatalf("shutdown error leaked: %v", err)
				}
			},
		)
	}
}

func TestDesktopStopHonorsDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := waitForDesktop(ctx, make(chan struct{})); !errors.Is(err, context.Canceled) {
		t.Fatalf("stop did not honor cancellation: %v", err)
	}
	done := make(chan struct{})
	close(done)
	if err := waitForDesktop(t.Context(), done); err != nil {
		t.Fatal(err)
	}
}

func TestDesktopRequestWorkerUsesIndependentLifetime(t *testing.T) {
	runtime := lifetime.New()
	desktop := desktopwails.New(
		&desktopwails.Dependencies{Configuration: desktopConfiguration(configurationFixture())},
	)
	graph := fxtest.New(t, fx.Invoke(func(lc fx.Lifecycle) {
		lc.Append(desktopLifecycle(desktop, runtime, launch.New()))
	}))
	graph.RequireStart()
	if runtime.Context().Err() != nil {
		t.Fatal("startup ended the request worker lifetime")
	}
	graph.RequireStop()
	if runtime.Context().Err() == nil {
		t.Fatal("shutdown retained request worker lifetime")
	}
}

func configurationFixture() *configuration.Settings {
	settings, err := (configyaml.Loader{}).Load()
	if err != nil {
		panic(err)
	}
	return settings
}
