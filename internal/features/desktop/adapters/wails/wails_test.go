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
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	autostart "raven/internal/features/autostart/application"
	companion "raven/internal/features/companion/domain"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
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
func (backend *startupBackend) EnableWithOptions(string, []string) error {
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
	settings := nativeConfigurationFixture()
	app := NewApplication(&ApplicationDependencies{
		ApplicationID:  settings.ApplicationID,
		Logger:         logger,
		Middleware:     mediaRoutes{}.Middleware,
		SecondInstance: visibility.SecondInstance,
		Assets:         fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("Raven")}},
	})
	handles := NewWindows(app, settings)
	backend := &startupBackend{}
	startup := autostart.New(
		backend,
		startupSettings{},
		&autostart.Configuration{ApplicationID: settings.ApplicationID},
	)
	reminders := &reminderRecorder{}
	state := newDesktopState(&Dependencies{
		App:                        app,
		Main:                       handles.Main,
		Widget:                     handles.Widget,
		Review:                     handles.Review,
		ShowMain:                   windows.ShowMain,
		HideMain:                   windows.HideMain,
		ConfigureWidget:            windows.ConfigureWidget,
		PositionFloatingReview:     windows.PositionFloatingReview,
		ResetWidget:                windows.ResetWidget,
		RequestFloatingReviewClose: windows.RequestFloatingReviewClose,
		Quit:                       windows.Quit,
		ApplicationStarted:         visibility.ApplicationStarted,
		RequestOpen:                visibility.RequestOpen,
		InitializeStartup:          startup.Initialize,
		RefreshStartup:             startup.Refresh,
		ToggleStartup:              startup.Toggle,
		SetStartupPublisher:        startup.SetPublisher,
		StartReminders:             reminders.Start,
		WakeReminders:              reminders.Wake,
		OnReview: reminderwails.NewRuntime(
			notifications.New(),
			nil,
			func(context.Context, error) {},
		).OnReview,
		ShowReviews: "reviews",
		LogErrorKey: "error",
	})
	desktop := &desktopFixture{newDesktopOperations(state), state}
	var reviewAction func()
	state.OnReview = func(show func()) { reviewAction = show }
	desktop.invoke = func(callback func()) { callback() }
	dialogs := 0
	desktop.showDialog = func(dialog *application.MessageDialog) {
		dialogs++
		if dialog.Title != startupMenuTitle || dialog.Message != startupErrorMessage {
			t.Errorf("wrong startup dialog: %+v", dialog)
		}
	}
	desktop.Configure()
	if applicationOptions(
		&ApplicationDependencies{
			ApplicationID: settings.ApplicationID,
			Middleware:    mediaRoutes{}.Middleware,
			Assets:        fstest.MapFS{"index.html": &fstest.MapFile{}},
		},
	).Assets.Middleware(
		http.NotFoundHandler(),
	) == nil {
		t.Fatal("missing media middleware")
	}
	if len(desktop.subscriptions) != 11 {
		t.Fatalf("registered %d handlers", len(desktop.subscriptions))
	}
	applicationHandler(state, applicationStarted)(nil)
	widgetShown(desktop.Dependencies, nil)
	widgetMoved(desktop.Dependencies, nil)
	for _, closeWindow := range []func(*application.WindowEvent){windowHandler(state, reviewClosing), windowHandler(state, mainClosing), cancelClosing} {
		event := application.NewWindowEvent()
		closeWindow(event)
		if !event.IsCancelled() {
			t.Fatal("native close was not canceled")
		}
	}
	systemWoke(desktop.Dependencies, nil)
	becameActive(desktop.Dependencies, nil)
	mainFocused(desktop.Dependencies, nil)
	reopened(desktop.Dependencies, nil)
	visibility.Classify(true)
	menuHandler(state, openClicked)(nil)
	resetClicked(desktop.Dependencies, nil)
	quitClicked(desktop.Dependencies, nil)
	reviewAction()
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
	reportStartupError(desktop.Dependencies, errors.New("startup failed"))
	backend.failure = errors.New("refresh failed")
	refreshStartup(desktop.Dependencies)
	if dialogs != 1 || !strings.Contains(logs.String(), "could not refresh login startup status") {
		t.Fatal("startup failures were not reported")
	}
	backend.failure = nil
	toggled := make(chan struct{})
	startup.SetPublisher(func(bool, bool) { close(toggled) })
	toggleStartup(desktop.Dependencies, nil)
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
		&ApplicationDependencies{
			ApplicationID:  settings.ApplicationID,
			SecondInstance: visibility.SecondInstance,
		},
	)
	options.OnSecondInstanceLaunch(application.SecondInstanceData{Args: []string{"--autostart"}})
	options.OnSecondInstanceLaunch(application.SecondInstanceData{})
}

func TestOpenRequestsStopWithRuntime(t *testing.T) {
	windows := &windowRecorder{}
	desktop := New(&Dependencies{ShowMain: windows.ShowMain})
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
	main := mainWindowOptions(nativeConfigurationFixture())
	if main.Name != "raven" || main.Title != "Raven" || main.URL != "/" || !main.Hidden ||
		main.Width != 960 ||
		main.Height != 680 ||
		main.MinWidth != 620 ||
		main.MinHeight != nativeConfigurationFixture().ReviewHeight {
		t.Fatalf("main window changed: %+v", main)
	}
	if main.Mac.Appearance != application.NSAppearanceNameDarkAqua ||
		main.Mac.Backdrop != application.MacBackdropTranslucent {
		t.Fatal("main window appearance changed")
	}
	widget := widgetWindowOptions(
		nativeConfigurationFixture(),
	)
	if widget.Name != "raven-widget" || widget.URL != "/?view=widget" || widget.Hidden ||
		widget.Width != nativeConfigurationFixture().WidgetSize ||
		widget.MinWidth != nativeConfigurationFixture().WidgetSize ||
		widget.MaxWidth != nativeConfigurationFixture().WidgetSize ||
		widget.Height != nativeConfigurationFixture().WidgetSize ||
		widget.MinHeight != nativeConfigurationFixture().WidgetSize ||
		widget.MaxHeight != nativeConfigurationFixture().WidgetSize {
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
	review := reviewWindowOptions(
		nativeConfigurationFixture(),
	)
	if review.Name != "raven-review" || review.URL != "/?view=review" || !review.Hidden ||
		review.Width != nativeConfigurationFixture().ReviewWidth ||
		review.Height != nativeConfigurationFixture().ReviewHeight ||
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

func TestMainWindowAllowsVideoHoverPlaybackWithoutClick(t *testing.T) {
	main := mainWindowOptions(nativeConfigurationFixture())
	preference := main.Mac.WebviewPreferences.EnableAutoplayWithoutUserAction
	if !preference.IsSet() || !preference.Get() {
		t.Fatal("main webview requires a click before video hover playback")
	}
}

func configurationFixture() configuration.Settings {
	settings, err := configyaml.Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func TestConfiguredIdentityAndGeometryReachNativeOptions(t *testing.T) {
	settings := nativeConfigurationFixture()
	settings.ApplicationID = "custom.desktop"
	settings.WidgetSize, settings.ReviewWidth, settings.ReviewHeight = 64, 300, 400
	if singleInstanceOptions(
		&ApplicationDependencies{ApplicationID: settings.ApplicationID},
	).UniqueID != "custom.desktop" {
		t.Fatal("single-instance identity ignored configuration")
	}
	widget, review := widgetWindowOptions(settings), reviewWindowOptions(settings)
	if widget.Width != 64 || widget.Height != 64 || review.Width != 300 || review.Height != 400 {
		t.Fatal("window sizes ignored configuration")
	}
}

type desktopFixture struct {
	*Desktop
	*Dependencies
}

func nativeConfigurationFixture() *Configuration {
	layout := configuration.Layout(configurationFixture())
	return &Configuration{
		ApplicationID: "raven.tests", WidgetSize: layout.WidgetSize,
		ReviewWidth: layout.ReviewWidth, ReviewHeight: layout.ReviewHeight,
	}
}

type windowSubscriptions struct {
	kinds   []events.WindowEventType
	stopped int
}

func (source *windowSubscriptions) OnWindowEvent(
	kind events.WindowEventType,
	_ func(*application.WindowEvent),
) func() {
	source.kinds = append(source.kinds, kind)
	return func() { source.stopped++ }
}

type applicationSubscriptions struct {
	kinds   []events.ApplicationEventType
	stopped int
}

func (source *applicationSubscriptions) OnApplicationEvent(
	kind events.ApplicationEventType,
	_ func(*application.ApplicationEvent),
) func() {
	source.kinds = append(source.kinds, kind)
	return func() { source.stopped++ }
}

func TestNativeEventPortsReleaseEveryOwnedSubscription(t *testing.T) {
	state := &Dependencies{}
	windows, applications := &windowSubscriptions{}, &applicationSubscriptions{}
	configureWidgetEvents(state, windows)
	configureLifecycleEvents(state, applications, windows)
	if len(windows.kinds) != 4 || len(applications.kinds) != 4 || len(state.subscriptions) != 8 {
		t.Fatal("native subscriptions did not cross the supplied event ports")
	}
	Dispose(state)
	if windows.stopped != 4 || applications.stopped != 4 || state.subscriptions != nil {
		t.Fatal("disposing native events retained subscriptions")
	}
}

type languageMessages struct {
	changed   func()
	refreshes int
	disposed  bool
}

func (*languageMessages) Text(key string, count int) string { return "ar:" + key }
func (messages *languageMessages) Subscribe(listener func()) func() {
	messages.changed = listener
	return func() { messages.disposed = true }
}
func (messages *languageMessages) Refresh() { messages.refreshes++ }

func TestNativeLanguageRefreshPreservesMenuStateAndDisposes(t *testing.T) {
	app := application.New(application.Options{})
	handles := NewWindows(app, nativeConfigurationFixture())
	messages := &languageMessages{}
	desktop := &Dependencies{
		App:          app,
		Widget:       handles.Widget,
		Review:       handles.Review,
		Localization: messages,
		post:         func(callback func()) { callback() },
		invoke:       func(callback func()) { callback() },
	}
	desktop.SetStartupPublisher = func(func(bool, bool)) {}
	refreshMenus := configureMenus(desktop)
	refreshMenus()
	translateMenu(&Dependencies{}, nil)
	startup, labels := populateMenu(desktop, app.Menu.New())
	startup.SetChecked(true).SetEnabled(true)
	labels()
	if startup.Label() != "ar:startup" || !startup.Checked() {
		t.Fatal("lost startup state")
	}
	refreshed := 0
	observeNativeLanguage(desktop, func() { refreshed++ })
	messages.changed()
	if refreshed != 1 {
		t.Fatal("language observer did not refresh")
	}
	if nativeText(desktop, "open", "") != "ar:open" {
		t.Fatal("untranslated native label")
	}
	Dispose(desktop)
	if !messages.disposed {
		t.Fatal("language observer retained")
	}
	desktop.RefreshStartup = func() error { return nil }
	desktop.WakeReminders = func() {}
	systemWoke(desktop, nil)
	becameActive(desktop, nil)
	if messages.refreshes < 2 {
		t.Fatal("OS language was not refreshed")
	}
}
