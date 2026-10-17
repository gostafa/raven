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
	options.Assets.Middleware = func(next http.Handler) http.Handler { return deps.Media.Middleware(next) }
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

// New constructs desktop orchestration with complete, stable dependencies.
func New(deps *Dependencies) *Desktop {
	return &Desktop{
		run:           deps.App.Run,
		invoke:        application.InvokeSync,
		showDialog:    (*application.MessageDialog).Show,
		configuration: *deps.Configuration,
		app:           deps.App,
		mainWindow:    deps.Handles.Main,
		widget:        deps.Handles.Widget,
		reviewWindow:  deps.Handles.Review,
		windows:       deps.Windows,
		visibility:    deps.Visibility,
		startup:       deps.Startup,
		reminders:     deps.Reminders,
		notifications: deps.Notifications,
		icon:          deps.Icon,
		subscriptions: nil,
	}
}

// Configure registers native interaction handlers before running Wails.
func (desktop *Desktop) Configure() {
	desktop.configureWidgetEvents()
	desktop.configureClosingHooks()
	desktop.configureLifecycleEvents()
	desktop.configureMenus()
	desktop.notifications.OnReview(desktop.showReviews)
}

func (desktop *Desktop) showReviews() {
	desktop.windows.ShowMain()
	desktop.app.Event.Emit(desktop.configuration.ShowReviews)
}

// Emit publishes a frontend event through the desktop adapter.
func (desktop *EventPublisher) Emit(name string) { desktop.app.Event.Emit(name) }

// NewEvents creates the event boundary consumed by the facade and reminder worker.
func NewEvents(app *application.App) *EventPublisher { return &EventPublisher{app: app} }

// Run keeps the native event loop on the calling main goroutine.
func (desktop *Desktop) Run() error {
	err := desktop.run()
	if err != nil {
		return fmt.Errorf("run desktop application: %w", err)
	}

	return nil
}

// Register binds services before the native event loop starts.
//
//nolint:gocritic // Wails service bindings are passed by value.
func (desktop *Desktop) Register(
	service application.Service,
) {
	desktop.app.RegisterService(service)
}

// Open handles queued launch requests until the runtime context is canceled.
func (desktop *Desktop) Open(ctx context.Context, requests <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-requests:
			desktop.windows.ShowMain()
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
	options.MinHeight = configuration.Layout.ReviewHeight

	return options
}

func mainWindowOptions(configuration *Configuration) application.WebviewWindowOptions {
	options := mainWindowBounds(configuration)

	options.Name = appName
	options.Title = appTitle
	options.Hidden = true
	options.BackgroundColour = application.NewRGB(blackChannel, blackChannel, blackChannel)
	options.URL = "/"
	options.Mac = mainMacWindowOptions()

	return options
}

func mainMacWindowOptions() application.MacWindow {
	var options application.MacWindow

	options.Appearance = application.NSAppearanceNameDarkAqua
	options.Backdrop = application.MacBackdropTranslucent

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

	return options
}

func widgetWindowBounds(configuration *Configuration) application.WebviewWindowOptions {
	options := floatingWindowOptions()

	options.Width = configuration.Layout.WidgetSize
	options.Height = configuration.Layout.WidgetSize
	options.MinWidth = configuration.Layout.WidgetSize
	options.MinHeight = configuration.Layout.WidgetSize
	options.MaxWidth = configuration.Layout.WidgetSize
	options.MaxHeight = configuration.Layout.WidgetSize

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
		blackChannel,
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
	options.Width = configuration.Layout.ReviewWidth
	options.Height = configuration.Layout.ReviewHeight
	options.Hidden = true
	options.BackgroundColour = application.NewRGB(blackChannel, blackChannel, blackChannel)
	options.URL = "/?view=review"
	options.Mac = reviewMacWindowOptions()

	return options
}

func (desktop *Desktop) configureWidgetEvents() {
	service := desktop
	desktop.track(service.widget.OnWindowEvent(
		events.Common.WindowShow,
		service.widgetShown,
	))
	desktop.track(service.widget.OnWindowEvent(
		events.Common.WindowDidMove,
		service.widgetMoved,
	))
	desktop.track(service.widget.OnWindowEvent(
		events.Common.WindowDidResize,
		service.widgetShown,
	))
}

func (desktop *Desktop) configureClosingHooks() {
	service := desktop
	desktop.track(service.reviewWindow.RegisterHook(
		events.Common.WindowClosing,
		service.reviewClosing,
	))
	desktop.track(service.mainWindow.RegisterHook(
		events.Common.WindowClosing,
		service.mainClosing,
	))
	desktop.track(service.widget.RegisterHook(
		events.Common.WindowClosing,
		cancelClosing,
	))
}

func (desktop *Desktop) configureLifecycleEvents() {
	app := desktop.app
	desktop.track(
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, desktop.applicationStarted),
	)
	desktop.track(app.Event.OnApplicationEvent(
		events.Common.SystemDidWake,
		desktop.systemWoke,
	))
	desktop.track(app.Event.OnApplicationEvent(
		events.Mac.ApplicationShouldHandleReopen,
		desktop.reopened,
	))
	desktop.track(app.Event.OnApplicationEvent(
		events.Mac.ApplicationDidBecomeActive,
		desktop.becameActive,
	))
	desktop.track(desktop.mainWindow.OnWindowEvent(
		events.Common.WindowFocus,
		desktop.mainFocused,
	))
}

func (desktop *Desktop) applicationStarted(_ *application.ApplicationEvent) {
	service := desktop
	service.windows.ConfigureWidget()
	service.windows.ResetWidget()
	service.reminders.Start()
	desktop.visibility.ApplicationStarted()
	desktop.reportStartupError(desktop.startup.Initialize())
}

func (desktop *Desktop) reportStartupError(err error) {
	if err == nil {
		return
	}

	app := desktop.app
	app.Logger.ErrorContext(
		context.Background(),
		"could not configure login startup",
		//nolint:sloglint // Log field names come from validated embedded configuration.
		slog.Any(desktop.configuration.LogErrorKey, err),
	)
	desktop.showDialog(app.Dialog.Error().
		SetTitle(startupMenuTitle).
		SetMessage(startupErrorMessage + err.Error()))
}

func (desktop *Desktop) refreshStartup() {
	err := desktop.startup.Refresh()
	if err != nil {
		desktop.app.Logger.WarnContext(
			context.Background(),
			"could not refresh login startup status",
			//nolint:sloglint // Log field names come from validated embedded configuration.
			slog.Any(desktop.configuration.LogErrorKey, err),
		)
	}
}

func (desktop *Desktop) toggleStartup(_ *application.Context) {
	go func() { desktop.reportStartupError(desktop.startup.Toggle()) }()
}

func (desktop *Desktop) populateMenu(menu *application.Menu) *application.MenuItem {
	service := desktop

	menu.Add(openMenuTitle).OnClick(service.openClicked)
	menu.Add(resetMenuTitle).OnClick(service.resetClicked)
	menu.AddSeparator()

	startup := menu.AddCheckbox(startupMenuTitle, false).SetEnabled(false)
	startup.OnClick(desktop.toggleStartup)
	menu.AddSeparator()
	menu.Add(quitMenuTitle).OnClick(service.quitClicked)

	return startup
}

func (desktop *Desktop) configureMenus() {
	app := desktop.app
	menu := app.ContextMenu.New()
	widgetStartup := desktop.populateMenu(menu.Menu)
	app.ContextMenu.Add(appName, menu)

	trayMenu := app.Menu.New()
	trayStartup := desktop.populateMenu(trayMenu)
	tray := app.SystemTray.New().SetIcon(desktop.icon).SetMenu(trayMenu)
	tray.SetTooltip(appTitle)
	desktop.startup.SetPublisher(func(checked, available bool) {
		desktop.invoke(func() {
			widgetStartup.SetChecked(checked).SetEnabled(available)
			trayStartup.SetChecked(checked).SetEnabled(available)
			menu.Update()
			tray.SetMenu(trayMenu)
		})
	})
}

func (desktop *Desktop) widgetShown(
	_ *application.WindowEvent,
) {
	desktop.windows.ConfigureWidget()
}

func (desktop *Desktop) widgetMoved(_ *application.WindowEvent) {
	desktop.windows.PositionFloatingReview()
}

func (desktop *Desktop) reviewClosing(event *application.WindowEvent) {
	event.Cancel()
	desktop.windows.RequestFloatingReviewClose()
}

func (desktop *Desktop) mainClosing(event *application.WindowEvent) {
	event.Cancel()
	desktop.windows.HideMain()
}
func cancelClosing(event *application.WindowEvent) { event.Cancel() }
func (desktop *Desktop) systemWoke(_ *application.ApplicationEvent) {
	desktop.reminders.Wake()
	desktop.refreshStartup()
}

func (desktop *Desktop) reopened(
	_ *application.ApplicationEvent,
) {
	desktop.visibility.RequestOpen()
}
func (desktop *Desktop) becameActive(_ *application.ApplicationEvent) { desktop.refreshStartup() }
func (desktop *Desktop) mainFocused(_ *application.WindowEvent)       { desktop.refreshStartup() }
func (desktop *Desktop) openClicked(_ *application.Context)           { desktop.windows.ShowMain() }

func (desktop *Desktop) resetClicked(
	_ *application.Context,
) {
	desktop.windows.ResetWidget()
}
func (desktop *Desktop) quitClicked(_ *application.Context) { desktop.windows.Quit() }

func (desktop *Desktop) track(cancel func()) {
	desktop.subscriptions = append(desktop.subscriptions, cancel)
}

// Dispose releases event handlers owned by desktop orchestration.
func (desktop *Desktop) Dispose() {
	for index := range desktop.subscriptions {
		desktop.subscriptions[index]()
	}

	desktop.subscriptions = nil
}

func singleInstanceOptions(deps *ApplicationDependencies) *application.SingleInstanceOptions {
	var options application.SingleInstanceOptions

	options.UniqueID = deps.Configuration.ApplicationID
	options.OnSecondInstanceLaunch = func(data application.SecondInstanceData) {
		deps.Visibility.SecondInstance(data.Args)
	}

	return &options
}
