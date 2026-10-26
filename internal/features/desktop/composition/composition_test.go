package composition

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"testing/fstest"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	autostart "raven/internal/features/autostart/application"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
	desktopwails "raven/internal/features/desktop/adapters/wails"
	reminderwails "raven/internal/features/reminders/adapters/wails"
	cffbatch "raven/internal/shared/cffbatch/adapters/cff"
	mediawails "raven/internal/shared/desktopmedia/adapters/wails"
	"raven/internal/shared/lifetime"
)

func TestNativeDesktopProvidersAndLifecycle(t *testing.T) {
	settings := nativeTestConfiguration{
		Settings: configurationFixture(),
		id:       "raven.composition.tests.test_" + rand.Text(),
	}
	logger := slog.New(slog.NewTextHandler(t.Output(), nil))
	requests := autostart.NewRequests()
	runtime := lifetime.New()
	t.Cleanup(runtime.Cancel)
	app := newApplication(appParams{
		Media: mediawails.NewRegistry(
			cffbatch.New(),
			&mediawails.Dependencies{Logger: logger},
			&mediawails.Settings{},
		),
		Visibility: requests.Visibility(),
		Logger:     logger,
		Assets: Assets{
			Files: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Raven")}},
		},
	}, settings)
	lifecycle := fxtest.NewLifecycle(t)
	desktop := newDesktop(desktopParams{
		App:        app,
		Handles:    newWindows(app, settings),
		Lifecycle:  lifecycle,
		Runtime:    runtime,
		Requests:   requests,
		Visibility: requests.Visibility(),
		Startup: autostart.New(
			nil,
			nil,
			&autostart.Configuration{Debug: true, ApplicationID: settings.ApplicationID()},
		),
		Notifications: reminderwails.NewRuntime(
			notifications.New(),
			nil,
			func(context.Context, error) {},
		),
	}, settings)
	configureDesktop(desktop)
	lifecycle.RequireStart()
	lifecycle.RequireStop()
}

type nativeTestConfiguration struct {
	configuration.Settings
	id string
}

func (settings nativeTestConfiguration) ApplicationID() string { return settings.id }

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
	failure := errors.New("lifecycle failed")
	if isolated := isolate(
		failure,
	); isolated.Error() != failure.Error() ||
		errors.Unwrap(isolated) != nil {
		t.Fatalf("lifecycle error contract: %v", isolated)
	}
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
		&desktopwails.Dependencies{},
	)
	graph := fxtest.New(t, fx.Invoke(func(lc fx.Lifecycle) {
		lc.Append(desktopLifecycle(desktop, runtime, autostart.NewRequests()))
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

func configurationFixture() configuration.Settings {
	settings, err := configyaml.Load()
	if err != nil {
		panic(err)
	}
	return settings
}

type bindingWindows struct{ shown int }

func (windows *bindingWindows) ShowMain()           { windows.shown++ }
func (*bindingWindows) HideMain()                   {}
func (*bindingWindows) ConfigureWidget()            {}
func (*bindingWindows) ResetWidget()                {}
func (*bindingWindows) PositionFloatingReview()     {}
func (*bindingWindows) RequestFloatingReviewClose() {}
func (*bindingWindows) ToggleFloatingReview()       {}
func (*bindingWindows) SyncFloatingReview()         {}
func (*bindingWindows) HideFloatingReview()         {}
func (*bindingWindows) Quit()                       {}

type bindingReminders struct{ wakes int }

func (*bindingReminders) Start()          {}
func (*bindingReminders) Stop()           {}
func (reminders *bindingReminders) Wake() { reminders.wakes++ }

func TestDesktopCapabilitiesBindOnlyPresentFeatures(t *testing.T) {
	deps := &desktopwails.Dependencies{}
	bindDesktopWindows(deps, desktopParams{})
	bindDesktopReminders(deps, desktopParams{})
	if deps.ShowMain != nil || deps.WakeReminders != nil {
		t.Fatal("absent features acquired native callbacks")
	}
	windows, reminders := &bindingWindows{}, &bindingReminders{}
	params := desktopParams{Windows: windows, Reminders: reminders}
	bindDesktopWindows(deps, params)
	bindDesktopReminders(deps, params)
	deps.ShowMain()
	deps.WakeReminders()
	if windows.shown != 1 || reminders.wakes != 1 {
		t.Fatal("feature callbacks did not retain their owners")
	}
}
