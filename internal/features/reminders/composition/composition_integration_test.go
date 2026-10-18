package composition_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
	desktopports "raven/internal/features/desktop/ports"
	reminderwails "raven/internal/features/reminders/adapters/wails"
	"raven/internal/features/reminders/composition"
	reminderports "raven/internal/features/reminders/ports"
	boltstore "raven/internal/shared/boltstore/adapters/bbolt"
	storecomposition "raven/internal/shared/boltstore/composition"
	libraryports "raven/internal/shared/boltstore/ports"
	cffbatch "raven/internal/shared/cffbatch/adapters/cff"
	batchcomposition "raven/internal/shared/cffbatch/composition"
	mediacomposition "raven/internal/shared/desktopmedia/composition"
	"raven/internal/shared/lifetime"
	scheduling "raven/internal/shared/scheduling/adapters/fsrs"
	schedulingcomposition "raven/internal/shared/scheduling/composition"
)

type native struct {
	start    func(context.Context) error
	stop     func() error
	response func(notifications.NotificationResult)
}

func (n *native) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	return n.start(ctx)
}
func (n *native) ServiceShutdown() error                                 { return n.stop() }
func (*native) CheckNotificationAuthorization() (bool, error)            { return false, nil }
func (*native) SendNotification(notifications.NotificationOptions) error { return nil }
func (*native) RequestNotificationAuthorization() (bool, error)          { return false, nil }
func (n *native) OnNotificationResponse(callback func(notifications.NotificationResult)) {
	n.response = callback
}

type events struct{ published chan struct{} }

func (e *events) Emit(string) {
	select {
	case e.published <- struct{}{}:
	default:
	}
}

func modules(path string, n *native, e *events) fx.Option {
	return fx.Options(
		storecomposition.Module,
		mediacomposition.Module,
		schedulingcomposition.Module,
		batchcomposition.Module,
		fx.Supply(fx.Annotate(configurationFixture(), fx.As(new(configuration.Settings)))),
		composition.Module,
		fx.Provide(
			lifetime.New,
			func() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) },
		),
		fx.Supply(fx.Annotate(e, fx.As(new(desktopports.Events)))),
		fx.Replace(
			storecomposition.DatabasePath(path),
			fx.Annotate(n, fx.As(new(reminderwails.NativeRuntime))),
		),
	)
}

func waitPublished(t *testing.T, e *events) {
	t.Helper()
	select {
	case <-e.published:
	case <-time.After(time.Second):
		t.Fatal("reminder did not publish")
	}
}

func TestLifecycleOrderingReadinessAndLockRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	var repo libraryports.Repository
	var control reminderports.Control
	var notifier *reminderwails.Runtime
	var runtime *lifetime.Runtime
	e := &events{published: make(chan struct{}, 1)}
	starts, stops := 0, 0
	n := &native{start: func(ctx context.Context) error {
		starts++
		if _, err := repo.ListCards(); err != nil {
			t.Fatalf("notifications started before storage: %v", err)
		}
		if ctx != runtime.Context() {
			t.Fatal("notification startup used a transient context")
		}
		return nil
	}, stop: func() error {
		stops++
		if _, err := repo.ListCards(); err != nil {
			t.Errorf("storage closed before notifications: %v", err)
		}
		return nil
	}}
	graph := fxtest.New(t, modules(path, n, e), fx.Populate(&repo, &control, &notifier, &runtime))
	t.Cleanup(runtime.Cancel)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("graph construction opened storage: %v", err)
	}
	graph.RequireStart()
	t.Cleanup(func() { graph.RequireStop() })
	select {
	case <-e.published:
		t.Fatal("reminders started before native readiness")
	default:
	}
	control.Start()
	control.Start()
	waitPublished(t, e)
	if runtime.Context().Err() != nil {
		t.Fatal("runtime canceled after successful startup")
	}
	control.Wake()
	waitPublished(t, e)
	opened := 0
	notifier.OnReview(func() { opened++ })
	n.response(notifications.NotificationResult{Error: errors.New("response failure")})
	n.response(notifications.NotificationResult{})
	var response notifications.NotificationResult
	response.Response.ActionIdentifier = notifications.DefaultActionIdentifier
	n.response(response)
	if opened != 1 {
		t.Fatalf("opened reviews %d times", opened)
	}
	graph.RequireStop()
	if starts != 1 || stops != 1 {
		t.Fatalf("starts=%d stops=%d", starts, stops)
	}
	control.Start()
	control.Wake()
	reopened, err := boltstore.Open(
		path,
		boltstore.NewDependencies(
			cffbatch.New(),
			scheduling.New(new(configuration.Library(configurationFixture()))),
			storeSettingsFixture(),
		),
	)
	if err != nil {
		t.Fatalf("shutdown retained database lock: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestUnavailableNotificationsLeaveReconciliationAndPromptAvailable(t *testing.T) {
	var repo libraryports.Repository
	var control reminderports.Control
	e := &events{published: make(chan struct{}, 1)}
	n := &native{
		start: func(context.Context) error { return errors.New("unavailable") },
		stop:  func() error { t.Error("shut down uninitialized notifications"); return nil },
	}
	graph := fxtest.New(
		t,
		modules(filepath.Join(t.TempDir(), "raven.db"), n, e),
		fx.Populate(&repo, &control),
	)
	graph.RequireStart()
	t.Cleanup(func() { graph.RequireStop() })
	control.Start()
	waitPublished(t, e)
	claim, err := repo.ClaimNotificationPrompt()
	if err != nil || !claim {
		t.Fatalf("unavailable notifications consumed prompt: %v %v", claim, err)
	}
}

func TestPartialStartupRollsBackEveryStartedResource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	e := &events{published: make(chan struct{}, 1)}
	stops := 0
	n := &native{
		start: func(context.Context) error { return nil },
		stop:  func() error { stops++; return nil },
	}
	cause := errors.New("late startup failure")
	graph := fxtest.New(
		t,
		modules(path, n, e),
		fx.Invoke(func(lc fx.Lifecycle, _ reminderports.Control) {
			lc.Append(fx.Hook{OnStart: func(context.Context) error { return cause }})
		}),
	)
	if err := graph.Start(t.Context()); !errors.Is(err, cause) {
		t.Fatalf("startup error: %v", err)
	}
	graph.RequireStop()
	if stops != 1 {
		t.Fatalf("notifications stopped %d times", stops)
	}
	store, err := boltstore.Open(
		path,
		boltstore.NewDependencies(
			cffbatch.New(),
			scheduling.New(new(configuration.Library(configurationFixture()))),
			storeSettingsFixture(),
		),
	)
	if err != nil {
		t.Fatalf("rollback retained database lock: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseFailurePreventsNativeStartup(t *testing.T) {
	e := &events{published: make(chan struct{}, 1)}
	n := &native{
		start: func(context.Context) error { t.Error("notifications started without storage"); return nil },
		stop:  func() error { t.Error("notifications stopped without startup"); return nil },
	}
	graph := fxtest.New(t, modules(t.TempDir(), n, e), fx.Invoke(func(reminderports.Control) {}))
	if err := graph.Start(t.Context()); err == nil {
		t.Fatal("database startup unexpectedly succeeded")
	}
	graph.RequireStop()
}

func configurationFixture() configuration.Settings {
	settings, err := configyaml.Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func storeSettingsFixture() *boltstore.Settings {
	settings := configurationFixture()
	return &boltstore.Settings{
		Rules:            configuration.Library(settings),
		MediaRoutePrefix: settings.MediaRoutePrefix(),
		LogErrorKey:      settings.LogErrorKey(),
	}
}
