package wails

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	autostart "raven/internal/features/autostart/application"
	companion "raven/internal/features/companion/domain"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
	"raven/internal/features/desktop/ports"
	reminderwails "raven/internal/features/reminders/adapters/wails"
)

type windowRecorder struct {
	companion.Windows
	shown                                               atomic.Int32
	hidden, configured, positioned, reset, closed, quit int
}

func (windows *windowRecorder) ShowMain()                   { windows.shown.Add(1) }
func (windows *windowRecorder) HideMain()                   { windows.hidden++ }
func (windows *windowRecorder) ConfigureWidget()            { windows.configured++ }
func (windows *windowRecorder) PositionFloatingReview()     { windows.positioned++ }
func (windows *windowRecorder) ResetWidget()                { windows.reset++ }
func (windows *windowRecorder) RequestFloatingReviewClose() { windows.closed++ }
func (windows *windowRecorder) Quit()                       { windows.quit++ }

type reminderRecorder struct{ starts, wakes int }

func (reminders *reminderRecorder) Start() { reminders.starts++ }
func (reminders *reminderRecorder) Wake()  { reminders.wakes++ }
func (*reminderRecorder) Stop()            {}

type startupBackend struct {
	failure error
	enabled bool
}

func (backend *startupBackend) IsEnabled() (bool, error) { return backend.enabled, backend.failure }
func (backend *startupBackend) EnableWithOptions(*autostart.Options) error {
	backend.enabled = true
	return nil
}
func (backend *startupBackend) Disable() error { backend.enabled = false; return nil }

type startupSettings struct{}

func (startupSettings) ClaimAutostartInitialization() (bool, error) { return false, nil }

type mediaRoutes struct{}

func (mediaRoutes) Middleware(next http.Handler) http.HandlerFunc { return next.ServeHTTP }

func TestDesktopLifecycleAndNativeRegistrations(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	windows := &windowRecorder{}
	visibility := autostart.NewVisibility(windows.ShowMain)
	settings := &Configuration{
		ApplicationID: "raven.tests",
		LogErrorKey:   "error",
		ShowReviews:   "reviews",
		Layout:        configurationFixture().Layout,
	}
	app := NewApplication(&ApplicationDependencies{
		Configuration: settings,
		Logger:        logger,
		Media:         mediaRoutes{},
		Visibility:    visibility,
		Assets: ports.Assets{
			Files: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Raven")}},
		},
	})
	handles := NewWindows(app, settings)
	backend := &startupBackend{}
	startup := autostart.New(backend, false, settings.ApplicationID)
	reminders := &reminderRecorder{}
	desktop := New(&Dependencies{
		Configuration: settings, App: app, Handles: handles, Windows: windows,
		Visibility: visibility, Startup: startup, Settings: startupSettings{}, Reminders: reminders,
		Notifications: reminderwails.NewRuntime(notifications.New(), logger, "error"),
	})
	desktop.invoke = func(callback func()) { callback() }
	dialogs := 0
	desktop.showDialog = func(dialog *application.MessageDialog) {
		dialogs++
		if dialog.Title != startupMenuTitle || !strings.Contains(dialog.Message, "startup failed") {
			t.Errorf("wrong startup dialog: %+v", dialog)
		}
	}
	desktop.Configure()
	if applicationOptions(
		&ApplicationDependencies{
			Configuration: settings,
			Media:         mediaRoutes{},
			Assets:        ports.Assets{Files: fstest.MapFS{"index.html": &fstest.MapFile{}}},
		},
	).Assets.Middleware(
		http.NotFoundHandler(),
	) == nil {
		t.Fatal("missing media middleware")
	}
	if len(desktop.subscriptions) != 11 {
		t.Fatalf("registered %d handlers", len(desktop.subscriptions))
	}
	desktop.applicationStarted(nil)
	desktop.widgetShown(nil)
	desktop.widgetMoved(nil)
	for _, closeWindow := range []func(*application.WindowEvent){desktop.reviewClosing, desktop.mainClosing, cancelClosing} {
		event := application.NewWindowEvent()
		closeWindow(event)
		if !event.IsCancelled() {
			t.Fatal("native close was not canceled")
		}
	}
	desktop.systemWoke(nil)
	desktop.becameActive(nil)
	desktop.mainFocused(nil)
	desktop.reopened(nil)
	visibility.Classify(true)
	desktop.openClicked(nil)
	desktop.resetClicked(nil)
	desktop.quitClicked(nil)
	desktop.showReviews()
	NewEvents(app).Emit("test:event")
	if windows.configured != 2 || windows.positioned != 1 || windows.hidden != 1 ||
		windows.closed != 1 ||
		windows.reset != 2 ||
		windows.quit != 1 ||
		windows.shown.Load() != 3 ||
		reminders.starts != 1 ||
		reminders.wakes != 1 {
		t.Fatalf("unexpected window or reminder actions: %+v %+v", windows, reminders)
	}
	desktop.reportStartupError(errors.New("startup failed"))
	backend.failure = errors.New("refresh failed")
	desktop.refreshStartup()
	if dialogs != 1 || !strings.Contains(logs.String(), "could not refresh login startup status") {
		t.Fatal("startup failures were not reported")
	}
	backend.failure = nil
	toggled := make(chan struct{})
	startup.SetPublisher(func(bool, bool) { close(toggled) })
	desktop.toggleStartup(nil)
	select {
	case <-toggled:
	case <-time.After(time.Second):
		t.Fatal("startup menu toggle did not complete")
	}
	startup.SetPublisher(nil)
	desktop.Dispose()
	desktop.Dispose()
	if desktop.subscriptions != nil {
		t.Fatal("retained subscriptions")
	}
	desktop.Register(application.NewService(&struct{}{}))
	desktop.run = func() error { return nil }
	if err := desktop.Run(); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("event loop failed")
	desktop.run = func() error { return failure }
	if err := desktop.Run(); !errors.Is(err, failure) {
		t.Fatalf("run error: %v", err)
	}
	options := singleInstanceOptions(
		&ApplicationDependencies{Configuration: settings, Visibility: visibility},
	)
	options.OnSecondInstanceLaunch(application.SecondInstanceData{Args: []string{"--autostart"}})
	options.OnSecondInstanceLaunch(application.SecondInstanceData{})
}

func TestOpenRequestsStopWithRuntime(t *testing.T) {
	windows := &windowRecorder{}
	desktop := &Desktop{windows: windows}
	ctx, cancel := context.WithCancel(t.Context())
	requests := make(chan struct{})
	done := make(chan struct{})
	go func() { desktop.Open(ctx, requests); close(done) }()
	requests <- struct{}{}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request loop did not stop")
	}
	if windows.shown.Load() != 1 {
		t.Fatal("request did not show the library")
	}
}

func TestDesktopWindowConfigurationPreservesMonochromeBehavior(t *testing.T) {
	main := mainWindowOptions(&Configuration{Layout: configurationFixture().Layout})
	if main.Name != "raven" || main.Title != "Raven" || main.URL != "/" || !main.Hidden ||
		main.Width != 960 ||
		main.Height != 680 ||
		main.MinWidth != 620 ||
		main.MinHeight != 520 {
		t.Fatalf("main window changed: %+v", main)
	}
	if main.Mac.Appearance != application.NSAppearanceNameDarkAqua ||
		main.Mac.Backdrop != application.MacBackdropTranslucent {
		t.Fatal("main window appearance changed")
	}
	widget := widgetWindowOptions(&Configuration{Layout: configurationFixture().Layout})
	if widget.Name != "raven-widget" || widget.URL != "/?view=widget" || widget.Hidden ||
		widget.Width != configurationFixture().Layout.WidgetSize ||
		widget.MinWidth != configurationFixture().Layout.WidgetSize ||
		widget.MaxWidth != configurationFixture().Layout.WidgetSize ||
		widget.Height != configurationFixture().Layout.WidgetSize ||
		widget.MinHeight != configurationFixture().Layout.WidgetSize ||
		widget.MaxHeight != configurationFixture().Layout.WidgetSize {
		t.Fatal("widget geometry changed")
	}
	if widget.InitialPosition != application.WindowXY || widget.X != 24 || widget.Y != 100 ||
		widget.BackgroundType != application.BackgroundTypeTransparent ||
		!reflect.DeepEqual(widget.BackgroundColour, application.NewRGBA(255, 255, 255, 0)) {
		t.Fatal("widget background or placement changed")
	}
	if widget.Mac.Backdrop != application.MacBackdropTransparent || !widget.Mac.DisableShadow ||
		widget.Mac.CornerType != application.MacWindowCornerTypeSquare {
		t.Fatal("widget platform appearance changed")
	}
	review := reviewWindowOptions(&Configuration{Layout: configurationFixture().Layout})
	if review.Name != "raven-review" || review.URL != "/?view=review" || !review.Hidden ||
		review.Width != configurationFixture().Layout.ReviewWidth ||
		review.Height != configurationFixture().Layout.ReviewHeight ||
		review.Mac.Appearance != application.NSAppearanceNameDarkAqua {
		t.Fatal("review window changed")
	}
	for _, window := range []application.WebviewWindowOptions{widget, review} {
		if !window.Frameless || !window.DisableResize || !window.AlwaysOnTop ||
			!window.Windows.HiddenOnTaskbar ||
			window.MaximiseButtonState != application.ButtonDisabled ||
			window.FullscreenButtonState != application.ButtonDisabled {
			t.Fatal("floating window interaction changed")
		}
		if window.Mac.WindowClass != application.MacWindowClassPanel ||
			!window.Mac.PanelPreferences.NonActivating ||
			!window.Mac.PanelPreferences.FloatingPanel ||
			window.Mac.WindowLevel != application.MacWindowLevelPopUpMenu {
			t.Fatal("floating panel behavior changed")
		}
	}
	for _, window := range []application.WebviewWindowOptions{main, review} {
		if !reflect.DeepEqual(window.BackgroundColour, application.NewRGB(0, 0, 0)) {
			t.Fatal("native black background changed")
		}
	}
	identity := desktopIdentityOptions([]byte{1})
	if identity.Name != "Raven" ||
		identity.Description != "A little practice. A little more remembered." ||
		len(identity.Icon) == 0 {
		t.Fatal("application identity changed")
	}
}

func configurationFixture() *configuration.Settings {
	settings, err := (configyaml.Loader{}).Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func TestConfiguredIdentityAndGeometryReachNativeOptions(t *testing.T) {
	settings := &Configuration{
		ApplicationID: "custom.desktop",
		Layout:        configurationFixture().Layout,
	}
	settings.Layout.WidgetSize = 64
	settings.Layout.ReviewWidth = 300
	settings.Layout.ReviewHeight = 400
	if singleInstanceOptions(
		&ApplicationDependencies{Configuration: settings},
	).UniqueID != "custom.desktop" {
		t.Fatal("single-instance identity ignored configuration")
	}
	widget, review := widgetWindowOptions(settings), reviewWindowOptions(settings)
	if widget.Width != 64 || widget.Height != 64 || review.Width != 300 || review.Height != 400 {
		t.Fatal("window sizes ignored configuration")
	}
}
