// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	companionwails "raven/internal/features/companion/adapters/wails"
)

// NewApplication creates the single-instance shell before any persistence is opened.
func NewApplication(deps *ApplicationDependencies) *application.App {
	return application.New(applicationOptions(deps))
}

func applicationOptions(deps *ApplicationDependencies) application.Options {
	options := desktopIdentityOptions(deps.Icon)

	options.Logger = deps.Logger
	options.Assets.Handler = application.AssetFileServerFS(deps.Assets)
	options.Assets.Middleware = func(next http.Handler) http.Handler { return deps.Middleware(next) }
	options.Windows.DisableQuitOnLastWindowClosed = true
	options.Linux.DisableQuitOnLastWindowClosed = true
	options.SingleInstance = singleInstanceOptions(deps)

	return options
}

// NewWindows constructs all native window handles for injection into feature adapters.
func NewWindows(app *application.App, configuration *Configuration) companionwails.WindowSet {
	return companionwails.WindowSet{
		Main:   app.Window.NewWithOptions(mainWindowOptions(configuration)),
		Widget: app.Window.NewWithOptions(widgetWindowOptions(configuration)),
		Review: app.Window.NewWithOptions(reviewWindowOptions(configuration)),
	}
}

// New constructs native orchestration with stable operation capabilities.
func New(deps *Dependencies) *Desktop { return newDesktopOperations(newDesktopState(deps)) }

func newDesktopState(deps *Dependencies) *Dependencies {
	state := *deps

	state.run = deps.App.Run
	state.invoke = application.InvokeSync
	state.post = application.InvokeAsync
	state.showDialog = (*application.MessageDialog).Show

	return &state
}

func newDesktopOperations(state *Dependencies) *Desktop {
	operations := desktopLifecycleOperations(state)

	operations["configure"] = func(context.Context, *application.Service, <-chan struct{}) desktopResult {
		Configure(state)

		return desktopResult{failure: nil}
	}
	operations["register"] = func(_ context.Context, service *application.Service, _ <-chan struct{}) desktopResult {
		state.App.RegisterService(*service)

		return desktopResult{failure: nil}
	}

	return &Desktop{operations: operations}
}

func desktopLifecycleOperations(
	state *Dependencies,
) map[string]func(context.Context, *application.Service, <-chan struct{}) desktopResult {
	return map[string]func(context.Context, *application.Service, <-chan struct{}) desktopResult{
		"run": func(context.Context, *application.Service, <-chan struct{}) desktopResult {
			return desktopResult{failure: Run(state)}
		},
		"dispose": func(context.Context, *application.Service, <-chan struct{}) desktopResult {
			Dispose(state)

			return desktopResult{failure: nil}
		},
		"open": func(ctx context.Context, _ *application.Service, requests <-chan struct{}) desktopResult {
			Open(ctx, state, requests)

			return desktopResult{failure: nil}
		},
	}
}

// Configure registers native handlers before the event loop starts.
func (desktop *Desktop) Configure() { desktop.operations["configure"](context.Background(), nil, nil) }

// Run keeps the native event loop on the calling main goroutine.
func (desktop *Desktop) Run() error {
	return desktop.operations["run"](context.Background(), nil, nil).failure
}

// Register binds a Wails service before the native event loop starts.
//
//nolint:gocritic // Wails service bindings are passed by value.
func (desktop *Desktop) Register(service application.Service) {
	desktop.operations["register"](context.Background(), &service, nil)
}

// Open handles queued launch requests until cancellation.
func (desktop *Desktop) Open(ctx context.Context, requests <-chan struct{}) {
	desktop.operations["open"](ctx, nil, requests)
}

// Dispose releases native event subscriptions.
func (desktop *Desktop) Dispose() { desktop.operations["dispose"](context.Background(), nil, nil) }

// Configure registers native interaction handlers before running Wails.
func Configure(desktop *Dependencies) {
	configureWidgetEvents(desktop, desktop.Widget)
	configureClosingHooks(desktop)
	configureLifecycleEvents(desktop, desktop.App.Event, desktop.Main)
	observeNativeLanguage(desktop, configureMenus(desktop))
	desktop.OnReview(func() { showReviews(desktop) })
}

func showReviews(desktop *Dependencies) {
	desktop.ShowMain()
	desktop.App.Event.Emit(desktop.ShowReviews)
}

// Emit publishes a frontend event through the desktop adapter.
func (desktop *EventPublisher) Emit(name string) { desktop.app.Event.Emit(name) }

// NewEvents creates the event boundary consumed by the facade and reminder worker.
func NewEvents(app *application.App) *EventPublisher { return &EventPublisher{app: app} }

// Run keeps the native event loop on the calling main goroutine.
func Run(desktop *Dependencies) error {
	err := desktop.run()
	if err != nil {
		return fmt.Errorf("run desktop application: %w", err)
	}

	return nil
}

// Open handles queued launch requests until the runtime context is canceled.
func Open(ctx context.Context, desktop *Dependencies, requests <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-requests:
			desktop.ShowMain()
		}
	}
}

func desktopIdentityOptions(icon []byte) application.Options {
	var options application.Options

	options.Name = appTitle
	options.Description = "A little practice. A little more remembered."
	options.Icon = icon

	return options
}

func mainWindowBounds(configuration *Configuration) application.WebviewWindowOptions {
	var options application.WebviewWindowOptions

	options.Width = mainWindowWidth
	options.Height = mainWindowHeight
	options.MinWidth = mainWindowMinWidth
	options.MinHeight = configuration.ReviewHeight

	return options
}

func mainWindowOptions(configuration *Configuration) application.WebviewWindowOptions {
	options := mainWindowBounds(configuration)

	options.Name = appName
	options.Title = appTitle
	options.Hidden = true
	options.BackgroundColour = application.NewRGB(zero, zero, zero)
	options.URL = "/"
	options.Mac = mainMacWindowOptions()
	options.Windows.Theme = application.Dark

	return options
}

func mainMacWindowOptions() application.MacWindow {
	var options application.MacWindow

	options.Appearance = application.NSAppearanceNameDarkAqua
	options.Backdrop = application.MacBackdropTranslucent
	options.WebviewPreferences.EnableAutoplayWithoutUserAction.Set(true)

	return options
}

func floatingWindowOptions() application.WebviewWindowOptions {
	var options application.WebviewWindowOptions

	options.Frameless = true
	options.DisableResize = true
	options.AlwaysOnTop = true
	options.MaximiseButtonState = application.ButtonDisabled
	options.FullscreenButtonState = application.ButtonDisabled
	options.Windows.HiddenOnTaskbar = true
	options.Windows.Theme = application.Dark

	return options
}

func widgetWindowBounds(configuration *Configuration) application.WebviewWindowOptions {
	options := floatingWindowOptions()

	options.Width = configuration.WidgetSize
	options.Height = configuration.WidgetSize
	options.MinWidth = configuration.WidgetSize
	options.MinHeight = configuration.WidgetSize
	options.MaxWidth = configuration.WidgetSize
	options.MaxHeight = configuration.WidgetSize

	return options
}

func widgetWindowOptions(configuration *Configuration) application.WebviewWindowOptions {
	options := widgetWindowBounds(configuration)

	options.Name = "raven-widget"
	options.Title = "Raven widget"
	options.BackgroundType = application.BackgroundTypeTransparent
	options.BackgroundColour = application.NewRGBA(
		whiteChannel,
		whiteChannel,
		whiteChannel,
		zero,
	)
	options.URL = "/?view=widget"
	options.Mac = widgetMacWindowOptions()
	placeInitialWidget(&options)

	return options
}

func placeInitialWidget(options *application.WebviewWindowOptions) {
	options.InitialPosition = application.WindowXY
	options.X = initialWidgetX
	options.Y = initialWidgetY
}

func floatingMacWindowOptions() application.MacWindow {
	var options application.MacWindow

	options.WindowClass = application.MacWindowClassPanel
	options.PanelPreferences.NonActivating = true
	options.PanelPreferences.FloatingPanel = true
	options.WindowLevel = application.MacWindowLevelPopUpMenu
	options.CollectionBehavior = application.MacWindowCollectionBehaviorCanJoinAllSpaces |
		application.MacWindowCollectionBehaviorFullScreenAuxiliary |
		application.MacWindowCollectionBehaviorIgnoresCycle

	return options
}

func widgetMacWindowOptions() application.MacWindow {
	options := floatingMacWindowOptions()

	options.Backdrop = application.MacBackdropTransparent
	options.CornerType = application.MacWindowCornerTypeSquare
	options.DisableShadow = true

	return options
}

func reviewMacWindowOptions() application.MacWindow {
	options := floatingMacWindowOptions()

	options.Appearance = application.NSAppearanceNameDarkAqua

	return options
}

func reviewWindowOptions(configuration *Configuration) application.WebviewWindowOptions {
	options := floatingWindowOptions()

	options.Name = "raven-review"
	options.Title = "Raven review"
	options.Width = configuration.ReviewWidth
	options.Height = configuration.ReviewHeight
	options.Hidden = true
	options.BackgroundColour = application.NewRGB(zero, zero, zero)
	options.URL = "/?view=review"
	options.Mac = reviewMacWindowOptions()

	return options
}

func configureWidgetEvents(desktop *Dependencies, source windowEventSource) {
	service := desktop
	track(desktop, source.OnWindowEvent(
		events.Common.WindowShow,
		windowHandler(service, widgetShown),
	))
	track(desktop, source.OnWindowEvent(
		events.Common.WindowDidMove,
		windowHandler(service, widgetMoved),
	))
	track(desktop, source.OnWindowEvent(
		events.Common.WindowDidResize,
		windowHandler(service, widgetShown),
	))
}

func configureClosingHooks(desktop *Dependencies) {
	service := desktop
	track(desktop, service.Review.RegisterHook(
		events.Common.WindowClosing,
		windowHandler(service, reviewClosing),
	))
	track(desktop, service.Main.RegisterHook(
		events.Common.WindowClosing,
		windowHandler(service, mainClosing),
	))
	track(desktop, service.Widget.RegisterHook(
		events.Common.WindowClosing,
		cancelClosing,
	))
}

func configureLifecycleEvents(
	desktop *Dependencies,
	applications applicationEventSource,
	windows windowEventSource,
) {
	track(
		desktop,
		applications.OnApplicationEvent(
			events.Common.ApplicationStarted,
			applicationHandler(desktop, applicationStarted),
		),
	)
	track(desktop, applications.OnApplicationEvent(
		events.Common.SystemDidWake,
		applicationHandler(desktop, systemWoke),
	))
	track(desktop, applications.OnApplicationEvent(
		events.Mac.ApplicationShouldHandleReopen,
		applicationHandler(desktop, reopened),
	))
	configureFocusEvents(desktop, applications, windows)
}

func configureFocusEvents(
	desktop *Dependencies,
	applications applicationEventSource,
	windows windowEventSource,
) {
	track(desktop, applications.OnApplicationEvent(
		events.Mac.ApplicationDidBecomeActive,
		applicationHandler(desktop, becameActive),
	))
	track(desktop, windows.OnWindowEvent(
		events.Common.WindowFocus,
		windowHandler(desktop, mainFocused),
	))
}

func applicationStarted(desktop *Dependencies, _ *application.ApplicationEvent) {
	service := desktop
	refreshNativeLanguage(desktop)
	service.ConfigureWidget()
	service.ResetWidget()
	service.StartReminders()
	desktop.ApplicationStarted()
	reportStartupError(desktop, desktop.InitializeStartup())
}

func reportStartupError(desktop *Dependencies, err error) {
	if err == nil {
		return
	}

	app := desktop.App
	app.Logger.ErrorContext(
		context.Background(),
		"could not configure login startup",
		//nolint:sloglint // Log field names come from validated embedded configuration.
		slog.Any(desktop.LogErrorKey, err),
	)
	desktop.showDialog(app.Dialog.Error().
		SetTitle(nativeText(desktop, startupMessage, startupMenuTitle)).
		SetMessage(nativeText(desktop, "startupError", startupErrorMessage)))
}

func refreshStartup(desktop *Dependencies) {
	err := desktop.RefreshStartup()
	if err != nil {
		desktop.App.Logger.WarnContext(
			context.Background(),
			"could not refresh login startup status",
			//nolint:sloglint // Log field names come from validated embedded configuration.
			slog.Any(desktop.LogErrorKey, err),
		)
	}
}

func toggleStartup(desktop *Dependencies, _ *application.Context) {
	go func() { reportStartupError(desktop, desktop.ToggleStartup()) }()
}

func populateMenu(
	desktop *Dependencies,
	menu *application.Menu,
) (startupItem *application.MenuItem, refreshLabels func()) {
	labels := map[string]*application.MenuItem{
		openMessage:  addWindowMenuItem(desktop, menu, openMessage),
		resetMessage: addWindowMenuItem(desktop, menu, resetMessage),
	}
	menu.AddSeparator()

	startup := startupMenuItem(desktop, menu)

	labels[startupMessage] = startup

	menu.AddSeparator()

	labels[quitMessage] = addWindowMenuItem(desktop, menu, quitMessage)

	return startup, func() { translateMenu(desktop, labels) }
}

func addWindowMenuItem(
	desktop *Dependencies,
	menu *application.Menu,
	key string,
) *application.MenuItem {
	titles := map[string]string{
		openMessage:  openMenuTitle,
		resetMessage: resetMenuTitle,
		quitMessage:  quitMenuTitle,
	}
	handlers := map[string]func(*Dependencies, *application.Context){
		openMessage:  openClicked,
		resetMessage: resetClicked,
		quitMessage:  quitClicked,
	}

	return menu.Add(nativeText(desktop, key, titles[key])).
		OnClick(menuHandler(desktop, handlers[key]))
}

func startupMenuItem(desktop *Dependencies, menu *application.Menu) *application.MenuItem {
	return menu.AddCheckbox(nativeText(desktop, startupMessage, startupMenuTitle), false).
		SetEnabled(false).
		OnClick(menuHandler(desktop, toggleStartup))
}

func translateMenu(desktop *Dependencies, labels map[string]*application.MenuItem) {
	if desktop.Localization == nil {
		return
	}

	for key := range labels {
		labels[key].SetLabel(desktop.Localization.Text(key, zero))
	}
}

func configureMenus(desktop *Dependencies) func() {
	contextMenu := desktop.App.ContextMenu.New()
	widget, widgetLabels := populateMenu(desktop, contextMenu.Menu)
	desktop.App.ContextMenu.Add(appName, contextMenu)

	trayMenu := desktop.App.Menu.New()
	trayCheck, trayLabels := populateMenu(desktop, trayMenu)
	tray := newTray(desktop.App, trayMenu, desktop.Icon)
	refresh := menuRefresh(contextMenu, tray, trayMenu)
	bindStartupChecks(desktop, []*application.MenuItem{widget, trayCheck}, refresh)

	return func() { widgetLabels(); trayLabels(); refresh() }
}

func menuRefresh(
	contextMenu *application.ContextMenu,
	tray *application.SystemTray,
	menu *application.Menu,
) func() {
	return func() { contextMenu.Update(); tray.SetMenu(menu) }
}

func bindStartupChecks(desktop *Dependencies, items []*application.MenuItem, refresh func()) {
	desktop.SetStartupPublisher(func(checked, available bool) {
		desktop.invoke(func() {
			for index := range items {
				items[index].SetChecked(checked).SetEnabled(available)
			}

			refresh()
		})
	})
}

func newTray(app *application.App, menu *application.Menu, icon []byte) *application.SystemTray {
	tray := app.SystemTray.New().SetIcon(icon).SetMenu(menu)
	tray.SetTooltip(appTitle)

	return tray
}

func observeNativeLanguage(desktop *Dependencies, refresh func()) {
	if desktop.Localization == nil {
		return
	}

	track(
		desktop,
		desktop.Localization.Subscribe(
			func() { desktop.post(func() { refresh(); refreshNativeLanguage(desktop) }) },
		),
	)
}

func widgetShown(desktop *Dependencies,
	_ *application.WindowEvent,
) {
	desktop.ConfigureWidget()
}

func widgetMoved(desktop *Dependencies, _ *application.WindowEvent) {
	desktop.PositionFloatingReview()
}

func reviewClosing(desktop *Dependencies, event *application.WindowEvent) {
	event.Cancel()
	desktop.RequestFloatingReviewClose()
}

func mainClosing(desktop *Dependencies, event *application.WindowEvent) {
	event.Cancel()
	desktop.HideMain()
}
func cancelClosing(event *application.WindowEvent) { event.Cancel() }
func systemWoke(desktop *Dependencies, _ *application.ApplicationEvent) {
	if desktop.Localization != nil {
		desktop.Localization.Refresh()
	}

	desktop.WakeReminders()

	if desktop.Localization != nil {
		desktop.Localization.Refresh()
	}

	refreshStartup(desktop)
}

func reopened(desktop *Dependencies,
	_ *application.ApplicationEvent,
) {
	desktop.RequestOpen()
}

func becameActive(
	desktop *Dependencies,
	_ *application.ApplicationEvent,
) {
	if desktop.Localization != nil {
		desktop.Localization.Refresh()
	}

	refreshStartup(desktop)
}

func mainFocused(desktop *Dependencies, _ *application.WindowEvent) { refreshStartup(desktop) }
func openClicked(desktop *Dependencies, _ *application.Context)     { desktop.ShowMain() }

func resetClicked(desktop *Dependencies,
	_ *application.Context,
) {
	desktop.ResetWidget()
}
func quitClicked(desktop *Dependencies, _ *application.Context) { desktop.Quit() }

func track(desktop *Dependencies, cancel func()) {
	desktop.subscriptions = append(desktop.subscriptions, cancel)
}

// Dispose releases event handlers owned by desktop orchestration.
func Dispose(desktop *Dependencies) {
	for index := range desktop.subscriptions {
		desktop.subscriptions[index]()
	}

	desktop.subscriptions = nil
}

func singleInstanceOptions(deps *ApplicationDependencies) *application.SingleInstanceOptions {
	var options application.SingleInstanceOptions

	options.UniqueID = deps.ApplicationID
	options.OnSecondInstanceLaunch = func(data application.SecondInstanceData) {
		deps.SecondInstance(data.Args)
	}

	return &options
}

func windowHandler(
	state *Dependencies,
	handle func(*Dependencies, *application.WindowEvent),
) func(*application.WindowEvent) {
	return func(event *application.WindowEvent) { handle(state, event) }
}

func applicationHandler(
	state *Dependencies,
	handle func(*Dependencies, *application.ApplicationEvent),
) func(*application.ApplicationEvent) {
	return func(event *application.ApplicationEvent) { handle(state, event) }
}

func menuHandler(
	state *Dependencies,
	handle func(*Dependencies, *application.Context),
) func(*application.Context) {
	return func(event *application.Context) { handle(state, event) }
}

func nativeText(desktop *Dependencies, key, fallback string) string {
	if desktop.Localization == nil {
		return fallback
	}

	return desktop.Localization.Text(key, zero)
}

func refreshNativeLanguage(desktop *Dependencies) {
	if desktop.Localization == nil {
		return
	}

	desktop.Widget.SetTitle(desktop.Localization.Text("widgetTitle", zero))
	desktop.Review.SetTitle(desktop.Localization.Text("reviewTitle", zero))
}
