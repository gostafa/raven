package wails

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
)

type testWindow struct {
	fullscreen                          bool
	exitedFullscreen                    func(*application.WindowEvent)
	visible                             bool
	actions                             []string
	events                              []string
	horizontal, vertical, width, height int
	screen                              *application.Screen
	screenErr                           error
}

func (window *testWindow) IsFullscreen() bool { return window.fullscreen }

func (window *testWindow) UnFullscreen() { window.actions = append(window.actions, "exit-fullscreen") }

func (window *testWindow) OnWindowEvent(
	kind events.WindowEventType,
	callback func(*application.WindowEvent),
) func() {
	if kind == events.Common.WindowUnFullscreen {
		window.exitedFullscreen = callback
	}
	return func() {}
}

func TestClosingFullscreenMainWaitsForSpaceExit(t *testing.T) {
	windows, main, widget, _ := testWindows()
	main.visible, main.fullscreen = true, true
	windows.HideMain()
	windows.HideMain()
	if !main.visible || !reflect.DeepEqual(main.actions, []string{"exit-fullscreen"}) {
		t.Fatalf("hid the full-screen space before its exit completed: %+v", main)
	}
	main.fullscreen = false
	main.exitedFullscreen(nil)
	if main.visible || !widget.visible ||
		!reflect.DeepEqual(main.actions, []string{"exit-fullscreen", "hide"}) {
		t.Fatalf("did not finish closing after leaving full-screen: %+v", main)
	}
	main.exitedFullscreen(nil)
	if len(main.actions) != 2 {
		t.Fatal("repeated exit hid the window again")
	}
	windows.ShowMain()
	main.fullscreen = true
	windows.HideMain()
	main.fullscreen = false
	main.exitedFullscreen(nil)
	if main.visible {
		t.Fatal("a second full-screen close did not reuse the exit listener")
	}
}

func TestReopeningCancelsPendingFullscreenClose(t *testing.T) {
	windows, main, _, _ := testWindows()
	main.visible, main.fullscreen = true, true
	windows.HideMain()
	windows.ShowMain()
	main.fullscreen = false
	main.exitedFullscreen(nil)
	if !main.visible {
		t.Fatal("late full-screen exit hid the reopened library")
	}
}

type testConfiguration func() error

func (configure testConfiguration) Configure() error { return configure() }

func (window *testWindow) Show() application.Window {
	window.visible = true
	window.actions = append(window.actions, "show")
	return nil
}

func (window *testWindow) Hide() application.Window {
	window.visible = false
	window.actions = append(window.actions, "hide")
	return nil
}

func (window *testWindow) Restore()             { window.actions = append(window.actions, "restore") }
func (window *testWindow) Focus()               { window.actions = append(window.actions, "focus") }
func (window *testWindow) IsVisible() bool      { return window.visible }
func (window *testWindow) Position() (int, int) { return window.horizontal, window.vertical }
func (window *testWindow) SetPosition(horizontal, vertical int) {
	window.horizontal, window.vertical = horizontal, vertical
}

func (window *testWindow) SetSize(width, height int) application.Window {
	window.width, window.height = width, height
	return nil
}

func (window *testWindow) GetScreen() (*application.Screen, error) {
	return window.screen, window.screenErr
}

func (window *testWindow) DispatchWailsEvent(event *application.CustomEvent) {
	window.events = append(window.events, event.Name)
}

type windowFixture struct {
	*Windows
	*windowState
}

func testWindows() (*windowFixture, *testWindow, *testWindow, *testWindow) {
	main, widget, review := new(testWindow), new(testWindow), new(testWindow)
	state := &windowState{
		invoke:        func(operation func()) { operation() },
		primaryScreen: func() *application.Screen { return nil },
		quit:          func() {},
		configure:     func() error { return nil },
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	bindMainWindow(state, main)
	bindMainFullscreen(state, main)
	bindWidgetWindow(state, widget)
	bindReviewSurface(state, review)
	bindReviewFrame(state, review)
	bindWindowSettings(state, windowSettingsFixture())
	return &windowFixture{newWindowOperations(state), state}, main, widget, review
}

func TestWindowVisibilityAndReviewEvents(t *testing.T) {
	windows, main, widget, review := testWindows()
	windows.ShowMain()
	if !reflect.DeepEqual(main.actions, []string{"show", "restore", "focus"}) {
		t.Fatal(main.actions)
	}
	windows.HideMain()
	if main.visible || !widget.visible {
		t.Fatal("hiding main did not retain the widget")
	}
	windows.PositionFloatingReview()
	windows.ToggleFloatingReview()
	if widget.visible || !review.visible ||
		!reflect.DeepEqual(review.actions, []string{"show", "focus"}) ||
		!reflect.DeepEqual(review.events, []string{"raven:open-floating-review"}) {
		t.Fatalf("widget=%+v review=%+v", widget, review)
	}
	windows.ToggleFloatingReview()
	if !review.visible || review.events[len(review.events)-1] != "raven:close-floating-review" {
		t.Fatal("toggle did not defer close to the frontend")
	}
	windows.HideFloatingReview()
	if review.visible || !widget.visible {
		t.Fatal("review close did not restore the widget")
	}
	windows.ResetWidget()
	if !widget.visible {
		t.Fatal("reset did not show widget")
	}
	windows.reviewVisible = nil
	windows.PositionFloatingReview()
	quits := 0
	windows.quit = func() { quits++ }
	windows.Quit()
	if quits != 1 {
		t.Fatal("quit was not delegated")
	}
	windows.RequestFloatingReviewClose()
}

func TestReviewSynchronizationOnlyLoadsAnOpenWindow(t *testing.T) {
	windows, _, _, review := testWindows()
	windows.SyncFloatingReview()
	if len(review.events) != 0 || review.visible {
		t.Fatal("hidden startup review received an opening event")
	}
	windows.ToggleFloatingReview()
	review.events = nil
	review.actions = nil
	windows.SyncFloatingReview()
	if !reflect.DeepEqual(review.events, []string{"raven:open-floating-review"}) ||
		len(review.actions) != 0 {
		t.Fatal("frontend readiness did not recover an already open review", review)
	}
	windows.HideFloatingReview()
	review.events = nil
	windows.SyncFloatingReview()
	if len(review.events) != 0 {
		t.Fatal("closed review was reopened by synchronization")
	}
}

func TestReviewPlacementUsesWidgetScreenAndFallsBackToPrimary(t *testing.T) {
	windows, _, widget, review := testWindows()
	review.visible = true
	screen := &application.Screen{
		WorkArea: application.Rect{X: 100, Y: 200, Width: 1000, Height: 800},
	}
	widget.screen = screen
	widget.horizontal, widget.vertical = 1000, 900
	primaryCalls := 0
	windows.primaryScreen = func() *application.Screen { primaryCalls++; return screen }
	windows.PositionFloatingReview()
	if primaryCalls != 0 || review.width <= 0 || review.height <= 0 ||
		review.horizontal < 100 || review.vertical < 200 ||
		review.horizontal+review.width > 1100 || review.vertical+review.height > 1000 {
		t.Fatalf("review was not placed in the widget screen: %+v", review)
	}
	widget.screenErr = errors.New("screen unavailable")
	windows.PositionFloatingReview()
	if primaryCalls != 1 {
		t.Fatal("screen error did not fall back to primary")
	}
	widget.screenErr, widget.screen = nil, nil
	windows.PositionFloatingReview()
	if primaryCalls != 2 {
		t.Fatal("missing screen did not fall back to primary")
	}
	windows.ResetWidget()
	if widget.horizontal != 1028 || widget.vertical != 928 {
		t.Fatalf("unexpected widget corner: %+v", widget)
	}
	for _, invalid := range []*application.Screen{nil, {}, {WorkArea: application.Rect{Width: 100}}, {WorkArea: application.Rect{Height: 100}}} {
		windows.primaryScreen = func() *application.Screen { return invalid }
		review.width, review.height = 0, 0
		windows.PositionFloatingReview()
		if review.width != 0 || review.height != 0 {
			t.Fatal("placed review in unavailable work area")
		}
	}
}

func TestNativeWidgetConfigurationUsesDispatcherAndReportsFailure(t *testing.T) {
	windows, _, _, _ := testWindows()
	var output bytes.Buffer
	windows.logger = slog.New(slog.NewTextHandler(&output, nil))
	dispatched, configured := 0, 0
	windows.invoke = func(operation func()) { dispatched++; operation() }
	failure := errors.New("native styling failed")
	windows.configure = testConfiguration(func() error {
		configured++
		return failure
	})
	windows.ConfigureWidget()
	if dispatched != 1 || configured != 1 || !strings.Contains(output.String(), failure.Error()) {
		t.Fatal("native failure was not dispatched and reported")
	}
	windows.configure = testConfiguration(func() error { return nil })
	output.Reset()
	windows.ConfigureWidget()
	if output.Len() != 0 {
		t.Fatal("successful native configuration logged an error")
	}
}

func TestConstructorBindsNativeWindowsAndRuntime(t *testing.T) {
	app := &application.App{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	main, widget, review := new(
		application.WebviewWindow,
	), new(
		application.WebviewWindow,
	), new(
		application.WebviewWindow,
	)
	configured := 0
	deps := &Dependencies{
		App: app, Main: main, Widget: widget, Review: review,
		Configure: func() error { configured++; return nil },
	}
	state := newWindowState(deps, windowSettingsFixture())
	if state.showMain == nil || state.showWidget == nil || state.showReview == nil ||
		state.configure == nil || state.invoke == nil || state.primaryScreen == nil || state.quit == nil || state.logger != app.Logger {
		t.Fatal("constructor did not bind window capabilities")
	}
	state.invoke = func(operation func()) { operation() }
	newWindowOperations(state).ConfigureWidget()
	if configured != 1 {
		t.Fatal("constructor did not retain supplied widget configuration")
	}
	deps.Review = nil
	state = newWindowState(deps, windowSettingsFixture())
	if state.reviewVisible != nil || state.setReviewSize != nil {
		t.Fatal("missing review window retained capabilities")
	}
	New(deps, windowSettingsFixture()).PositionFloatingReview()
}

func configurationFixture() configuration.Settings {
	settings, err := configyaml.Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func windowSettingsFixture() *Settings {
	settings := configurationFixture()
	return &Settings{
		WidgetSize:          configuration.Layout(settings).WidgetSize,
		ReviewWidth:         configuration.Layout(settings).ReviewWidth,
		ReviewHeight:        configuration.Layout(settings).ReviewHeight,
		ReviewGap:           configuration.Layout(settings).ReviewGap,
		LogErrorKey:         settings.LogErrorKey(),
		OpenFloatingReview:  settings.OpenFloatingReview(),
		CloseFloatingReview: settings.CloseFloatingReview(),
	}
}

func TestConfiguredEventsGeometryAndLogFieldsReachConsumers(t *testing.T) {
	windows, _, widget, review := testWindows()
	windows.openFloatingReview = "custom:open"
	windows.closeFloatingReview = "custom:close"
	windows.logErrorKey = "custom_error"
	windows.reviewWidth = 300
	windows.reviewHeight = 400
	widget.screen = &application.Screen{WorkArea: application.Rect{Width: 1000, Height: 800}}
	windows.ToggleFloatingReview()
	windows.ToggleFloatingReview()
	if !reflect.DeepEqual(review.events, []string{"custom:open", "custom:close"}) ||
		review.width != 300 ||
		review.height != 400 {
		t.Fatal("configured events or geometry were ignored", review)
	}
	var output bytes.Buffer
	windows.logger = slog.New(slog.NewTextHandler(&output, nil))
	windows.configure = testConfiguration(func() error { return errors.New("styling failed") })
	windows.ConfigureWidget()
	if !strings.Contains(output.String(), "custom_error=") {
		t.Fatal(output.String())
	}
}
