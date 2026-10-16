package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	ns := notifications.New()
	raven := &RavenService{notifier: ns}
	visibility := &launchVisibility{show: raven.ShowMain}
	prepareNativeLoginDetection(func(nativeAutomatic bool) {
		// Never wait on the UI thread from inside a Cocoa notification.
		go visibility.classify(nativeAutomatic || isAutostartLaunch(os.Args[1:]))
	})
	app := application.New(application.Options{
		Name:        "Raven",
		Description: "A little practice. A little more remembered.",
		Icon:        appIcon,
		Services:    []application.Service{application.NewService(raven)},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: raven.mediaMiddleware,
		},
		Mac:     application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
		Windows: application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		Linux:   application.LinuxOptions{DisableQuitOnLastWindowClosed: true},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: applicationID,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				visibility.secondInstance(data.Args)
			},
		},
	})
	ns.OnNotificationResponse(func(result notifications.NotificationResult) {
		if result.Error == nil &&
			result.Response.ActionIdentifier == notifications.DefaultActionIdentifier {
			raven.ShowReviews()
		}
	})
	raven.app = app
	startup := &autostartController{backend: app.Autostart, debug: app.Env.Info().Debug}
	reportStartupError := func(err error) {
		if err != nil {
			app.Logger.Error("Could not configure login startup", "error", err)
			app.Dialog.Error().
				SetTitle("Start at login").
				SetMessage("Raven could not update login startup. You can try again from the Raven menu.\n\n" + err.Error()).
				Show()
		}
	}
	refreshStartup := func() {
		if err := startup.refresh(); err != nil {
			app.Logger.Warn("Could not refresh login startup status", "error", err)
		}
	}
	toggleStartup := func(_ *application.Context) {
		go func() { reportStartupError(startup.toggle()) }()
	}
	raven.mainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "raven", Title: "Raven",
		Width: 960, Height: 680, MinWidth: 620, MinHeight: 520,
		Hidden:           true,
		BackgroundColour: application.NewRGB(0, 0, 0),
		URL:              "/",
		Mac: application.MacWindow{
			Appearance: application.NSAppearanceNameDarkAqua,
			Backdrop:   application.MacBackdropTranslucent,
		},
	})
	raven.widget = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                  "raven-widget",
		Title:                 "Raven widget",
		Width:                 widgetSize,
		Height:                widgetSize,
		MinWidth:              widgetSize,
		MinHeight:             widgetSize,
		MaxWidth:              widgetSize,
		MaxHeight:             widgetSize,
		Frameless:             true,
		DisableResize:         true,
		AlwaysOnTop:           true,
		MaximiseButtonState:   application.ButtonDisabled,
		FullscreenButtonState: application.ButtonDisabled,
		BackgroundType:        application.BackgroundTypeTransparent,
		BackgroundColour:      application.NewRGBA(255, 255, 255, 0),
		URL:                   "/?view=widget",
		InitialPosition:       application.WindowXY,
		X:                     24,
		Y:                     100,
		Windows:               application.WindowsWindow{HiddenOnTaskbar: true},
		Mac: application.MacWindow{
			Backdrop:      application.MacBackdropTransparent,
			CornerType:    application.MacWindowCornerTypeSquare,
			DisableShadow: true,
			WindowClass:   application.MacWindowClassPanel,
			PanelPreferences: application.MacPanelPreferences{
				NonActivating: true, FloatingPanel: true,
			},
			WindowLevel: application.MacWindowLevelPopUpMenu,
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary |
				application.MacWindowCollectionBehaviorIgnoresCycle,
		},
	})
	raven.reviewWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                  "raven-review",
		Title:                 "Raven review",
		Width:                 reviewWidth,
		Height:                reviewHeight,
		Hidden:                true,
		Frameless:             true,
		DisableResize:         true,
		AlwaysOnTop:           true,
		MaximiseButtonState:   application.ButtonDisabled,
		FullscreenButtonState: application.ButtonDisabled,
		BackgroundColour:      application.NewRGB(0, 0, 0),
		URL:                   "/?view=review",
		Windows:               application.WindowsWindow{HiddenOnTaskbar: true},
		Mac: application.MacWindow{
			Appearance:  application.NSAppearanceNameDarkAqua,
			WindowClass: application.MacWindowClassPanel,
			PanelPreferences: application.MacPanelPreferences{
				NonActivating: true,
				FloatingPanel: true,
			},
			WindowLevel: application.MacWindowLevelPopUpMenu,
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary |
				application.MacWindowCollectionBehaviorIgnoresCycle,
		},
	})
	raven.widget.OnWindowEvent(events.Common.WindowShow, func(_ *application.WindowEvent) {
		raven.configureWidget()
	})
	raven.widget.OnWindowEvent(events.Common.WindowDidMove, func(_ *application.WindowEvent) {
		raven.positionFloatingReview()
	})
	raven.reviewWindow.RegisterHook(
		events.Common.WindowClosing,
		func(event *application.WindowEvent) {
			event.Cancel()
			raven.requestFloatingReviewClose()
		},
	)
	raven.widget.OnWindowEvent(events.Common.WindowDidResize, func(_ *application.WindowEvent) {
		raven.configureWidget()
	})

	// Closing a window leaves the companion alive; explicit Quit still exits.
	raven.mainWindow.RegisterHook(
		events.Common.WindowClosing,
		func(event *application.WindowEvent) {
			event.Cancel()
			raven.HideMain()
		},
	)
	raven.widget.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
	})
	app.Event.OnApplicationEvent(
		events.Common.ApplicationStarted,
		func(_ *application.ApplicationEvent) {
			raven.configureWidget()
			raven.ResetWidget()
			raven.startReminders()
			visibility.applicationStarted()
			reportStartupError(startup.initialize(raven.store))
		},
	)
	app.Event.OnApplicationEvent(
		events.Common.SystemDidWake,
		func(_ *application.ApplicationEvent) {
			raven.RefreshReminders()
			refreshStartup()
		},
	)
	app.Event.OnApplicationEvent(
		events.Mac.ApplicationShouldHandleReopen,
		func(_ *application.ApplicationEvent) {
			visibility.requestOpen()
		},
	)
	app.Event.OnApplicationEvent(
		events.Mac.ApplicationDidBecomeActive,
		func(_ *application.ApplicationEvent) { refreshStartup() },
	)
	raven.mainWindow.OnWindowEvent(
		events.Common.WindowFocus,
		func(_ *application.WindowEvent) { refreshStartup() },
	)

	menu := app.ContextMenu.New()
	menu.Add("Open Raven").OnClick(func(_ *application.Context) { raven.ShowMain() })
	menu.Add("Reset widget position").OnClick(func(_ *application.Context) { raven.ResetWidget() })
	menu.AddSeparator()
	widgetStartup := menu.AddCheckbox("Start at login", false).SetEnabled(false)
	widgetStartup.OnClick(toggleStartup)
	menu.AddSeparator()
	menu.Add("Quit Raven").OnClick(func(_ *application.Context) { raven.Quit() })
	app.ContextMenu.Add("raven", menu)

	trayMenu := app.Menu.New()
	trayMenu.Add("Open Raven").OnClick(func(_ *application.Context) { raven.ShowMain() })
	trayMenu.Add("Reset widget position").
		OnClick(func(_ *application.Context) { raven.ResetWidget() })
	trayMenu.AddSeparator()
	trayStartup := trayMenu.AddCheckbox("Start at login", false).SetEnabled(false)
	trayStartup.OnClick(toggleStartup)
	trayMenu.AddSeparator()
	trayMenu.Add("Quit Raven").OnClick(func(_ *application.Context) { raven.Quit() })
	tray := app.SystemTray.New().SetIcon(appIcon).SetMenu(trayMenu)
	tray.SetTooltip("Raven")
	startup.publish = func(checked, available bool) {
		application.InvokeSync(func() {
			widgetStartup.SetChecked(checked).SetEnabled(available)
			trayStartup.SetChecked(checked).SetEnabled(available)
			menu.Update()
			tray.SetMenu(trayMenu)
		})
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
