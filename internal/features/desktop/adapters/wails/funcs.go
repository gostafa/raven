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

	autostart "raven/internal/features/autostart/application"
	companionwails "raven/internal/features/companion/adapters/wails"
)

// NewApplication creates the single-instance shell before any persistence is opened.
func NewApplication(deps *ApplicationDependencies) *application.App {
	options := desktopIdentityOptions(deps.Assets.Icon)

	options.Logger = deps.Logger
	options.Assets.Handler = application.AssetFileServerFS(deps.Assets.Files)
	options.Assets.Middleware = func(next http.Handler) http.Handler { return deps.Media.Middleware(next) }
	options.Windows.DisableQuitOnLastWindowClosed = true
	options.Linux.DisableQuitOnLastWindowClosed = true
	options.SingleInstance = singleInstanceOptions(deps)

	return application.New(options)
}

// NewWindows constructs all native window handles for injection into feature adapters.
func NewWindows(app *application.App) companionwails.WindowSet {
	return companionwails.WindowSet{
		Main:   app.Window.NewWithOptions(mainWindowOptions()),
		Widget: app.Window.NewWithOptions(widgetWindowOptions()),
		Review: app.Window.NewWithOptions(reviewWindowOptions()),
	}
}

// New constructs desktop orchestration with complete, stable dependencies.
func New(deps *Dependencies) *Desktop {
	return &Desktop{
		app: deps.App, mainWindow: deps.Handles.Main, widget: deps.Handles.Widget,
		reviewWindow: deps.Handles.Review, windows: deps.Windows, store: deps.Settings,
		visibility: deps.Visibility, startup: deps.Startup, reminders: deps.Reminders,
		notifications: deps.Notifications, icon: deps.Assets.Icon, subscriptions: nil,
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
	desktop.app.Event.Emit("raven:show-reviews")
}

// Emit publishes a frontend event through the desktop adapter.
func (desktop *EventPublisher) Emit(name string) { desktop.app.Event.Emit(name) }

// NewEvents creates the event boundary consumed by the facade and reminder worker.
func NewEvents(app *application.App) *EventPublisher { return &EventPublisher{app: app} }

// Run keeps the native event loop on the calling main goroutine.
func (desktop *Desktop) Run() error {
	err := desktop.app.Run()
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

func mainWindowBounds() application.WebviewWindowOptions {
	var options application.WebviewWindowOptions

	options.Width = mainWindowWidth
	options.Height = mainWindowHeight
	options.MinWidth = mainWindowMinWidth
	options.MinHeight = reviewHeight

	return options
}

func mainWindowOptions() application.WebviewWindowOptions {
	options := mainWindowBounds()

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

func widgetWindowBounds() application.WebviewWindowOptions {
	options := floatingWindowOptions()

	options.Width = widgetSize
	options.Height = widgetSize
	options.MinWidth = widgetSize
	options.MinHeight = widgetSize
	options.MaxWidth = widgetSize
	options.MaxHeight = widgetSize

	return options
}

func widgetWindowOptions() application.WebviewWindowOptions {
	options := widgetWindowBounds()

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

func reviewWindowOptions() application.WebviewWindowOptions {
	options := floatingWindowOptions()

	options.Name = "raven-review"
	options.Title = "Raven review"
	options.Width = reviewWidth
	options.Height = reviewHeight
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
		func(_ *application.WindowEvent) { service.windows.ConfigureWidget() },
	))
	desktop.track(service.widget.OnWindowEvent(
		events.Common.WindowDidMove,
		func(_ *application.WindowEvent) { service.windows.PositionFloatingReview() },
	))
	desktop.track(service.widget.OnWindowEvent(
		events.Common.WindowDidResize,
		func(_ *application.WindowEvent) { service.windows.ConfigureWidget() },
	))
}

func (desktop *Desktop) configureClosingHooks() {
	service := desktop
	desktop.track(service.reviewWindow.RegisterHook(
		events.Common.WindowClosing,
		func(event *application.WindowEvent) { event.Cancel(); service.windows.RequestFloatingReviewClose() },
	))
	desktop.track(service.mainWindow.RegisterHook(
		events.Common.WindowClosing,
		func(event *application.WindowEvent) { event.Cancel(); service.windows.HideMain() },
	))
	desktop.track(service.widget.RegisterHook(
		events.Common.WindowClosing,
		func(event *application.WindowEvent) { event.Cancel() },
	))
}

func (desktop *Desktop) configureLifecycleEvents() {
	app := desktop.app
	desktop.track(
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, desktop.applicationStarted),
	)
	desktop.track(app.Event.OnApplicationEvent(
		events.Common.SystemDidWake,
		func(_ *application.ApplicationEvent) { desktop.reminders.Wake(); desktop.refreshStartup() },
	))
	desktop.track(app.Event.OnApplicationEvent(
		events.Mac.ApplicationShouldHandleReopen,
		func(_ *application.ApplicationEvent) { desktop.visibility.RequestOpen() },
	))
	desktop.track(app.Event.OnApplicationEvent(
		events.Mac.ApplicationDidBecomeActive,
		func(_ *application.ApplicationEvent) { desktop.refreshStartup() },
	))
	desktop.track(desktop.mainWindow.OnWindowEvent(
		events.Common.WindowFocus,
		func(_ *application.WindowEvent) { desktop.refreshStartup() },
	))
}

func (desktop *Desktop) applicationStarted(_ *application.ApplicationEvent) {
	service := desktop
	service.windows.ConfigureWidget()
	service.windows.ResetWidget()
	service.reminders.Start()
	desktop.visibility.ApplicationStarted()
	desktop.reportStartupError(desktop.startup.Initialize(service.store))
}

func (desktop *Desktop) reportStartupError(err error) {
	if err == nil {
		return
	}

	app := desktop.app
	app.Logger.ErrorContext(
		context.Background(),
		"could not configure login startup",
		slog.Any(logErrorKey, err),
	)
	app.Dialog.Error().
		SetTitle(startupMenuTitle).
		SetMessage(startupErrorMessage + err.Error()).
		Show()
}

func (desktop *Desktop) refreshStartup() {
	err := desktop.startup.Refresh()
	if err != nil {
		desktop.app.Logger.WarnContext(
			context.Background(),
			"could not refresh login startup status",
			slog.Any(logErrorKey, err),
		)
	}
}

func (desktop *Desktop) toggleStartup(_ *application.Context) {
	go func() { desktop.reportStartupError(desktop.startup.Toggle()) }()
}

func (desktop *Desktop) populateMenu(menu *application.Menu) *application.MenuItem {
	service := desktop

	menu.Add(openMenuTitle).OnClick(func(_ *application.Context) { service.windows.ShowMain() })
	menu.Add(resetMenuTitle).OnClick(func(_ *application.Context) { service.windows.ResetWidget() })
	menu.AddSeparator()

	startup := menu.AddCheckbox(startupMenuTitle, false).SetEnabled(false)
	startup.OnClick(desktop.toggleStartup)
	menu.AddSeparator()
	menu.Add(quitMenuTitle).OnClick(func(_ *application.Context) { service.windows.Quit() })

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
		application.InvokeSync(func() {
			widgetStartup.SetChecked(checked).SetEnabled(available)
			trayStartup.SetChecked(checked).SetEnabled(available)
			menu.Update()
			tray.SetMenu(trayMenu)
		})
	})
}

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

	options.UniqueID = autostart.ApplicationID
	options.OnSecondInstanceLaunch = func(data application.SecondInstanceData) {
		deps.Visibility.SecondInstance(data.Args)
	}

	return &options
}
