// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"raven/internal/autostart"
	loginnative "raven/internal/autostart/adapters/native"
	startupwails "raven/internal/autostart/adapters/wails"
	companionwails "raven/internal/companion/adapters/wails"
	"raven/internal/library"
	"raven/internal/library/adapters/boltstore"
	"raven/internal/library/adapters/desktopmedia"
	"raven/internal/library/adapters/scheduling"
	"raven/internal/reminders"
	reminderwails "raven/internal/reminders/adapters/wails"
)

func (r *RavenService) startReminders() {
	r.reminderMu.Lock()
	defer r.reminderMu.Unlock()

	if r.store == nil || r.reminders != nil {
		return
	}

	var notifier reminders.Notifier

	if r.notifier != nil {
		notifier = reminderwails.New(r.notifier)
	}

	r.reminders = reminders.Start(
		r.store,
		notifier,
		func() { r.app.Event.Emit("raven:schedules-changed") },
		func(message string, err error) { r.app.Logger.Warn(message, "error", err) },
	)
}

func (r *RavenService) configureWidget() { r.windows.ConfigureWidget() }

func (l *lifecycleAdapter) ServiceStartup(
	ctx context.Context,
	options application.ServiceOptions,
) error {
	r := l.service

	path, err := boltstore.DefaultPath()
	if err != nil {
		return publicError(err)
	}

	store, err := boltstore.Open(path, scheduling.New())
	if err != nil {
		return publicError(err)
	}

	r.store = store
	r.mediaHandler = store.MediaHandler()

	if r.notifier != nil {
		err := r.notifier.ServiceStartup(ctx, options)
		if err != nil {
			if r.app != nil {
				r.app.Logger.Warn("Desktop notifications unavailable", "error", err)
			}

			r.notifier = nil
		}
	}

	return nil
}

func (l *lifecycleAdapter) ServiceShutdown() error {
	r := l.service
	r.reminderMu.Lock()

	c := r.reminders
	r.reminderMu.Unlock()

	if c != nil {
		c.Stop()
	}

	if r.store == nil {
		return nil
	}

	if r.notifier != nil {
		_ = r.notifier.ServiceShutdown()
	}

	return r.store.Close()
}

func (r *RavenService) ListFolders() ([]Folder, error) {
	if r.store == nil {
		return nil, &ServiceError{Message: "folder storage is unavailable"}
	}

	value, err := r.store.ListFolders()

	return publicFolders(value), publicError(err)
}

func (r *RavenService) CreateFolder(input CreateFolderInput) (Folder, error) {
	if r.store == nil {
		return Folder{}, &ServiceError{Message: "folder storage is unavailable"}
	}

	value, err := r.store.CreateFolder(folderInput(input))

	return publicFolder(value), publicError(err)
}

func (r *RavenService) ListCards() ([]Card, error) {
	if r.store == nil {
		return nil, &ServiceError{Message: "card storage is unavailable"}
	}

	value, err := r.store.ListCards()

	return publicCards(value), publicError(err)
}

func (r *RavenService) DeleteFolder(folderID string) error {
	if r.store == nil {
		return &ServiceError{Message: "folder storage is unavailable"}
	}

	err := r.store.DeleteFolder(folderID)
	if err == nil {
		r.schedulesChanged()
	}

	return publicError(err)
}

func (r *RavenService) DeleteCard(folderID, cardID string) error {
	if r.store == nil {
		return &ServiceError{Message: "card storage is unavailable"}
	}

	err := r.store.DeleteCard(folderID, cardID)
	if err == nil {
		r.schedulesChanged()
	}

	return publicError(err)
}

func (r *RavenService) CreateCard(input CreateCardInput) (Card, error) {
	if r.store == nil {
		return Card{}, &ServiceError{Message: "card storage is unavailable"}
	}

	card, err := r.store.CreateCard(createCardInput(input))
	if err == nil {
		r.schedulesChanged()
	}

	return publicCard(card), publicError(err)
}

func (r *RavenService) UpdateCard(input UpdateCardInput) (Card, error) {
	if r.store == nil {
		return Card{}, &ServiceError{Message: "card storage is unavailable"}
	}

	card, err := r.store.UpdateCard(updateCardInput(input))
	if err == nil {
		r.schedulesChanged()
	}

	return publicCard(card), publicError(err)
}

func (r *RavenService) ReviewItem(
	folderID, cardID, itemID string,
	rating int,
) (Card, error) {
	if r.store == nil {
		return Card{}, &ServiceError{Message: "card storage is unavailable"}
	}

	card, err := r.store.ReviewItem(folderID, cardID, itemID, rating)
	if err == nil {
		r.schedulesChanged()
	}

	return publicCard(card), publicError(err)
}

func (r *RavenService) ShowMain() { r.windows.ShowMain() }

func (r *RavenService) HideMain() { r.windows.HideMain() }

func (r *RavenService) ResetWidget() { r.windows.ResetWidget() }

func (r *RavenService) Quit() { r.windows.Quit() }

func (r *RavenService) ShowReviews() {
	r.ShowMain()
	r.app.Event.Emit("raven:show-reviews")
}

func (r *RavenService) RefreshReminders() {
	r.reminderMu.Lock()
	defer r.reminderMu.Unlock()

	if r.reminders != nil {
		r.reminders.Wake()
	}
}

func (r *RavenService) schedulesChanged() {
	if r.app != nil {
		r.app.Event.Emit("raven:schedules-changed")
	}

	r.RefreshReminders()
}

func main() {
	ns := notifications.New()
	raven := &RavenService{notifier: ns}
	visibility := autostart.NewVisibility(raven.ShowMain)

	loginnative.PrepareLoginDetection(func(nativeAutomatic bool) {
		// Never wait on the UI thread from inside a Cocoa notification.
		go visibility.Classify(nativeAutomatic || autostart.IsAutostartLaunch(os.Args[1:]))
	})

	app := application.New(application.Options{
		Name:        "Raven",
		Description: "A little practice. A little more remembered.",
		Icon:        appIcon,
		Services: []application.Service{
			application.NewService(&lifecycleAdapter{service: raven}),
			application.NewService(raven),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: raven.mediaMiddleware,
		},
		Mac:     application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
		Windows: application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		Linux:   application.LinuxOptions{DisableQuitOnLastWindowClosed: true},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: autostart.ApplicationID,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				visibility.SecondInstance(data.Args)
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

	startup := autostart.New(startupwails.New(app.Autostart), app.Env.Info().Debug, nil)
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
		err := startup.Refresh()
		if err != nil {
			app.Logger.Warn("Could not refresh login startup status", "error", err)
		}
	}
	toggleStartup := func(_ *application.Context) {
		go func() { reportStartupError(startup.Toggle()) }()
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
	raven.windows = companionwails.New(app, raven.mainWindow, raven.widget, raven.reviewWindow)
	raven.media = desktopmedia.New(
		app,
		raven.mainWindow,
		func() http.Handler { return raven.mediaHandler },
	)
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
			visibility.ApplicationStarted()
			reportStartupError(startup.Initialize(raven.store))
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
			visibility.RequestOpen()
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
	startup.SetPublisher(func(checked, available bool) {
		application.InvokeSync(func() {
			widgetStartup.SetChecked(checked).SetEnabled(available)
			trayStartup.SetChecked(checked).SetEnabled(available)
			menu.Update()
			tray.SetMenu(trayMenu)
		})
	})

	err := app.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func (e *ServiceError) Error() string { return e.Message }

func publicError(err error) error {
	if err == nil {
		return nil
	}

	return &ServiceError{Message: err.Error()}
}

func mapValues[From, To any](values []From, convert func(From) To) []To {
	if values == nil {
		return nil
	}

	result := make([]To, len(values))
	for i, value := range values {
		result[i] = convert(value)
	}

	return result
}
func publicFolder(v library.Folder) Folder      { return Folder(v) }
func publicFolders(v []library.Folder) []Folder { return mapValues(v, publicFolder) }
func publicReview(v library.ItemReview) ItemReview {
	return ItemReview{Log: ReviewLog(v.Log), Automatic: v.Automatic}
}

func publicItem(v library.ContentItem) ContentItem {
	return ContentItem{
		Schedule:        Schedule(v.Schedule),
		Reviews:         mapValues(v.Reviews, publicReview),
		LastNotifiedDue: v.LastNotifiedDue,
		ID:              v.ID,
		FolderID:        v.FolderID,
		CardID:          v.CardID,
		Position:        v.Position,
		Kind:            v.Kind,
		Text:            v.Text,
		OriginalName:    v.OriginalName,
		MIMEType:        v.MIMEType,
		ByteSize:        v.ByteSize,
		RelativePath:    v.RelativePath,
	}
}

func publicCard(v library.Card) Card {
	return Card{
		ID:           v.ID,
		FolderID:     v.FolderID,
		CreatedAt:    v.CreatedAt,
		DueAt:        v.DueAt,
		LastReviewed: v.LastReviewed,
		Items:        mapValues(v.Items, publicItem),
	}
}
func publicCards(v []library.Card) []Card { return mapValues(v, publicCard) }

func folderInput(
	v CreateFolderInput,
) library.CreateFolderInput {
	return library.CreateFolderInput(v)
}
func itemInput(v ItemDraft) library.ItemDraft { return library.ItemDraft(v) }
func createCardInput(v CreateCardInput) library.CreateCardInput {
	return library.CreateCardInput{FolderID: v.FolderID, Items: mapValues(v.Items, itemInput)}
}

func updateCardInput(v UpdateCardInput) library.UpdateCardInput {
	return library.UpdateCardInput{
		FolderID: v.FolderID,
		CardID:   v.CardID,
		Items:    mapValues(v.Items, itemInput),
	}
}

func publicMedia(v library.SelectedMedia) SelectedMedia { return SelectedMedia(v) }
func (r *RavenService) SelectMediaFiles(kind string) ([]SelectedMedia, error) {
	if r.media == nil {
		return nil, &ServiceError{Message: "file selection is unavailable"}
	}

	values, err := r.media.Select(kind)

	return mapValues(values, publicMedia), publicError(err)
}

func (r *RavenService) mediaMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Composition initializes the picker before Run; tests can omit native windows.
		media := r.media
		if media == nil {
			media = desktopmedia.New(nil, nil, func() http.Handler { return r.mediaHandler })
		}

		media.Middleware(next).ServeHTTP(w, req)
	})
}

func (r *RavenService) positionFloatingReview()     { r.windows.PositionFloatingReview() }
func (r *RavenService) requestFloatingReviewClose() { r.windows.RequestFloatingReviewClose() }
func (r *RavenService) ToggleFloatingReview()       { r.windows.ToggleFloatingReview() }
func (r *RavenService) HideFloatingReview()         { r.windows.HideFloatingReview() }
