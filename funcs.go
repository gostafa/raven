// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package main

import (
	"context"
	"errors"
	"log/slog"
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

func (service *RavenService) startReminders() {
	service.reminderMu.Lock()
	defer service.reminderMu.Unlock()

	if service.store == nil || service.reminders != nil {
		return
	}

	var notifier reminders.Notifier

	if service.notifier != nil {
		notifier = reminderwails.New(service.notifier)
	}

	service.reminders = reminders.Start(
		service.store,
		notifier,
		func() { service.app.Event.Emit("raven:schedules-changed") },
		func(message string, err error) {
			service.app.Logger.WarnContext(
				context.Background(),
				"review reminder unavailable",
				slog.String(logOperationKey, message),
				slog.Any(logErrorKey, err),
			)
		},
	)
}

func (service *RavenService) configureWidget() { service.windows.ConfigureWidget() }

func (lifecycle *lifecycleAdapter) ServiceStartup(
	ctx context.Context,
	options application.ServiceOptions,
) error {
	service := lifecycle.service

	path, err := boltstore.DefaultPath()
	if err != nil {
		return &ServiceError{Message: err.Error()}
	}

	store, err := boltstore.Open(path, scheduling.New())
	if err != nil {
		return &ServiceError{Message: err.Error()}
	}

	service.store = store
	service.mediaHandler = store.MediaHandler()

	if service.notifier != nil {
		serviceStartupErr := service.notifier.ServiceStartup(ctx, options)
		if serviceStartupErr != nil {
			if service.app != nil {
				service.app.Logger.WarnContext(
					context.Background(),
					"desktop notifications unavailable",
					slog.Any(logErrorKey, serviceStartupErr),
				)
			}

			service.notifier = nil
		}
	}

	return nil
}

func (lifecycle *lifecycleAdapter) ServiceShutdown() error {
	lifecycle.service.stopReminders()

	return lifecycle.service.closeResources()
}

func (service *RavenService) stopReminders() {
	service.reminderMu.Lock()

	coordinator := service.reminders
	service.reminderMu.Unlock()

	if coordinator != nil {
		coordinator.Stop()
	}
}

func (service *RavenService) closeResources() error {
	var failure error

	if service.notifier != nil {
		failure = service.notifier.ServiceShutdown()
	}

	if service.store != nil {
		failure = errors.Join(failure, service.store.Close())
	}

	if failure != nil {
		return &ServiceError{Message: failure.Error()}
	}

	return nil
}

// ListFolders returns folders in creation order.
func (service *RavenService) ListFolders() ([]Folder, error) {
	if service.store == nil {
		return nil, &ServiceError{Message: "folder storage is unavailable"}
	}

	value, err := service.store.ListFolders()
	if err != nil {
		return publicFolders(value), &ServiceError{Message: err.Error()}
	}

	return publicFolders(value), nil
}

// CreateFolder creates a folder from editable metadata.
func (service *RavenService) CreateFolder(input CreateFolderInput) (Folder, error) {
	if service.store == nil {
		return Folder{}, &ServiceError{Message: "folder storage is unavailable"}
	}

	value, err := service.store.CreateFolder(folderInput(input))
	if err != nil {
		return publicFolder(value), &ServiceError{Message: err.Error()}
	}

	return publicFolder(value), nil
}

// ListCards returns the persisted library cards.
func (service *RavenService) ListCards() ([]Card, error) {
	if service.store == nil {
		return nil, &ServiceError{Message: "card storage is unavailable"}
	}

	value, err := service.store.ListCards()
	if err != nil {
		return publicCards(value), &ServiceError{Message: err.Error()}
	}

	return publicCards(value), nil
}

// DeleteFolder deletes a folder and its content.
func (service *RavenService) DeleteFolder(folderID string) error {
	if service.store == nil {
		return &ServiceError{Message: "folder storage is unavailable"}
	}

	err := service.store.DeleteFolder(folderID)
	if err == nil {
		service.schedulesChanged()
	}

	if err != nil {
		return &ServiceError{Message: err.Error()}
	}

	return nil
}

// DeleteCard deletes a card from its owning folder.
func (service *RavenService) DeleteCard(folderID, cardID string) error {
	if service.store == nil {
		return &ServiceError{Message: "card storage is unavailable"}
	}

	err := service.store.DeleteCard(folderID, cardID)
	if err == nil {
		service.schedulesChanged()
	}

	if err != nil {
		return &ServiceError{Message: err.Error()}
	}

	return nil
}

// CreateCard creates a card and initializes its item schedules.
func (service *RavenService) CreateCard(input CreateCardInput) (Card, error) {
	if service.store == nil {
		return Card{}, &ServiceError{Message: "card storage is unavailable"}
	}

	card, err := service.store.CreateCard(createCardInput(input))
	if err == nil {
		service.schedulesChanged()
	}

	if err != nil {
		return publicCard(card), &ServiceError{Message: err.Error()}
	}

	return publicCard(card), nil
}

// UpdateCard updates card content while preserving retained item schedules.
func (service *RavenService) UpdateCard(input UpdateCardInput) (Card, error) {
	if service.store == nil {
		return Card{}, &ServiceError{Message: "card storage is unavailable"}
	}

	card, err := service.store.UpdateCard(updateCardInput(input))
	if err == nil {
		service.schedulesChanged()
	}

	if err != nil {
		return publicCard(card), &ServiceError{Message: err.Error()}
	}

	return publicCard(card), nil
}

// ReviewItem records a rating and advances an item schedule.
func (service *RavenService) ReviewItem(
	folderID, cardID, itemID string,
	rating int,
) (Card, error) {
	if service.store == nil {
		return Card{}, &ServiceError{Message: "card storage is unavailable"}
	}

	card, err := service.store.ReviewItem(folderID, cardID, itemID, rating)
	if err == nil {
		service.schedulesChanged()
	}

	if err != nil {
		return publicCard(card), &ServiceError{Message: err.Error()}
	}

	return publicCard(card), nil
}

// ShowMain shows and focuses the main library window.
func (service *RavenService) ShowMain() { service.windows.ShowMain() }

// HideMain hides the main library window.
func (service *RavenService) HideMain() { service.windows.HideMain() }

// ResetWidget moves the floating widget onto the primary screen.
func (service *RavenService) ResetWidget() { service.windows.ResetWidget() }

// Quit terminates Raven.
func (service *RavenService) Quit() { service.windows.Quit() }

// ShowReviews opens the library review session.
func (service *RavenService) ShowReviews() {
	service.ShowMain()
	service.app.Event.Emit("raven:show-reviews")
}

// RefreshReminders requests reconciliation of persisted review schedules.
func (service *RavenService) RefreshReminders() {
	service.reminderMu.Lock()
	defer service.reminderMu.Unlock()

	if service.reminders != nil {
		service.reminders.Wake()
	}
}

func (service *RavenService) schedulesChanged() {
	if service.app != nil {
		service.app.Event.Emit("raven:schedules-changed")
	}

	service.RefreshReminders()
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
			app.Logger.ErrorContext(
				context.Background(),
				"could not configure login startup",
				slog.Any(logErrorKey, err),
			)
			app.Dialog.Error().
				SetTitle("Start at login").
				SetMessage("Raven could not update login startup. You can try again from the Raven menu.\n\n" + err.Error()).
				Show()
		}
	}
	refreshStartup := func() {
		err := startup.Refresh()
		if err != nil {
			app.Logger.WarnContext(
				context.Background(),
				"could not refresh login startup status",
				slog.Any(logErrorKey, err),
			)
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
	menu.Add(resetMenuTitle).OnClick(func(_ *application.Context) { raven.ResetWidget() })
	menu.AddSeparator()

	widgetStartup := menu.AddCheckbox("Start at login", false).SetEnabled(false)
	widgetStartup.OnClick(toggleStartup)
	menu.AddSeparator()
	menu.Add("Quit Raven").OnClick(func(_ *application.Context) { raven.Quit() })
	app.ContextMenu.Add("raven", menu)

	trayMenu := app.Menu.New()
	trayMenu.Add("Open Raven").OnClick(func(_ *application.Context) { raven.ShowMain() })
	trayMenu.Add(resetMenuTitle).
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
		app.Logger.ErrorContext(
			context.Background(),
			"application stopped",
			slog.Any(logErrorKey, err),
		)
		os.Exit(1)
	}
}

// Error returns the public service error message.
func (failure *ServiceError) Error() string { return failure.Message }

func mapValues[From, To any](values []From, convert func(From) To) []To {
	if values == nil {
		return nil
	}

	result := make([]To, 0, len(values))
	for _, value := range values {
		result = append(result, convert(value))
	}

	return result
}
func publicFolder(value library.Folder) Folder      { return Folder(value) }
func publicFolders(value []library.Folder) []Folder { return mapValues(value, publicFolder) }
func publicReview(value library.ItemReview) ItemReview {
	return ItemReview{Log: ReviewLog(value.Log), Automatic: value.Automatic}
}

func publicItem(value library.ContentItem) ContentItem {
	return ContentItem{
		Schedule:        Schedule(value.Schedule),
		Reviews:         mapValues(value.Reviews, publicReview),
		LastNotifiedDue: value.LastNotifiedDue,
		ID:              value.ID,
		FolderID:        value.FolderID,
		CardID:          value.CardID,
		Position:        value.Position,
		Kind:            value.Kind,
		Text:            value.Text,
		OriginalName:    value.OriginalName,
		MIMEType:        value.MIMEType,
		ByteSize:        value.ByteSize,
		RelativePath:    value.RelativePath,
	}
}

func publicCard(value library.Card) Card {
	return Card{
		ID:           value.ID,
		FolderID:     value.FolderID,
		CreatedAt:    value.CreatedAt,
		DueAt:        value.DueAt,
		LastReviewed: value.LastReviewed,
		Items:        mapValues(value.Items, publicItem),
	}
}
func publicCards(value []library.Card) []Card { return mapValues(value, publicCard) }

func folderInput(
	value CreateFolderInput,
) library.CreateFolderInput {
	return library.CreateFolderInput(value)
}
func itemInput(value ItemDraft) library.ItemDraft { return library.ItemDraft(value) }
func createCardInput(value CreateCardInput) library.CreateCardInput {
	return library.CreateCardInput{
		FolderID: value.FolderID,
		Items:    mapValues(value.Items, itemInput),
	}
}

func updateCardInput(value UpdateCardInput) library.UpdateCardInput {
	return library.UpdateCardInput{
		FolderID: value.FolderID,
		CardID:   value.CardID,
		Items:    mapValues(value.Items, itemInput),
	}
}

func publicMedia(value library.SelectedMedia) SelectedMedia { return SelectedMedia(value) }

// SelectMediaFiles selects local media for a card draft.
func (service *RavenService) SelectMediaFiles(kind string) ([]SelectedMedia, error) {
	if service.media == nil {
		return nil, &ServiceError{Message: "file selection is unavailable"}
	}

	values, err := service.media.Select(kind)
	if err != nil {
		return mapValues(values, publicMedia), &ServiceError{Message: err.Error()}
	}

	return mapValues(values, publicMedia), nil
}

func (service *RavenService) mediaMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		// Composition initializes the picker before Run; tests can omit native windows.
		media := service.media
		if media == nil {
			media = desktopmedia.New(nil, nil, func() http.Handler { return service.mediaHandler })
		}

		media.Middleware(next).ServeHTTP(writer, req)
	})
}

func (service *RavenService) positionFloatingReview() { service.windows.PositionFloatingReview() }
func (service *RavenService) requestFloatingReviewClose() {
	service.windows.RequestFloatingReviewClose()
}

// ToggleFloatingReview opens or requests closing the floating review window.
func (service *RavenService) ToggleFloatingReview() { service.windows.ToggleFloatingReview() }

// HideFloatingReview hides the floating review window.
func (service *RavenService) HideFloatingReview() { service.windows.HideFloatingReview() }
