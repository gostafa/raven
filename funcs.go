// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package main

import (
	"context"
	"errors"
	"fmt"
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

	service.reminders = reminders.Start(service.store, notifier, service.reminderCallbacks())
}

func (service *RavenService) reminderCallbacks() reminders.Callbacks {
	return reminders.Callbacks{
		Publish: func() { service.app.Event.Emit(ravenSchedulesChanged) },
		Warn:    service.reportReminderFailure,
	}
}

func (service *RavenService) reportReminderFailure(message string, err error) {
	service.app.Logger.WarnContext(
		context.Background(),
		"review reminder unavailable",
		slog.String(logOperationKey, message),
		slog.Any(logErrorKey, err),
	)
}

func (service *RavenService) configureWidget() { service.windows.ConfigureWidget() }

func (lifecycle *lifecycleAdapter) ServiceStartup(
	ctx context.Context,
	options application.ServiceOptions,
) error {
	session, err := lifecycle.openStore()
	if err != nil {
		return &ServiceError{Message: err.Error()}
	}

	lifecycle.service.store = session.repository
	lifecycle.service.mediaHandler = session.media
	lifecycle.service.startNotifications(ctx, &options)

	return nil
}

func openServiceStore() (storageSession, error) {
	path, err := boltstore.DefaultPath()
	if err != nil {
		return storageSession{}, fmt.Errorf("locate library: %w", err)
	}

	store, err := boltstore.Open(path, scheduling.New())
	if err != nil {
		return storageSession{}, fmt.Errorf("open library: %w", err)
	}

	return storageSession{repository: store, media: store.MediaHandler()}, nil
}

func (service *RavenService) startNotifications(
	ctx context.Context,
	options *application.ServiceOptions,
) {
	if service.notifier == nil {
		return
	}

	err := service.notifier.ServiceStartup(ctx, *options)
	if err != nil {
		service.reportNotificationFailure(ctx, err)

		service.notifier = nil
	}
}

func (service *RavenService) reportNotificationFailure(ctx context.Context, err error) {
	if service.app != nil {
		service.app.Logger.WarnContext(
			ctx,
			"desktop notifications unavailable",
			slog.Any(logErrorKey, err),
		)
	}
}

func (lifecycle *lifecycleAdapter) ServiceShutdown() error {
	lifecycle.service.stopReminders()

	err := lifecycle.service.closeResources()
	if err != nil {
		return &ServiceError{Message: err.Error()}
	}

	return nil
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
		return nil, &ServiceError{Message: folderStorageIsUnavailable}
	}

	value, err := service.store.ListFolders()
	if err != nil {
		return publicFolders(value), &ServiceError{Message: err.Error()}
	}

	return publicFolders(value), nil
}

// CreateFolder creates a folder from editable metadata.
func (service *RavenService) CreateFolder(input *CreateFolderInput) (Folder, error) {
	if service.store == nil {
		return Folder{}, &ServiceError{Message: folderStorageIsUnavailable}
	}

	value, err := service.store.CreateFolder(folderInput(nonNilInput(input)))
	if err != nil {
		return publicFolder(&value), &ServiceError{Message: err.Error()}
	}

	return publicFolder(&value), nil
}

// ListCards returns the persisted library cards.
func (service *RavenService) ListCards() ([]Card, error) {
	if service.store == nil {
		return nil, &ServiceError{Message: cardStorageIsUnavailable}
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
		return &ServiceError{Message: folderStorageIsUnavailable}
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
		return &ServiceError{Message: cardStorageIsUnavailable}
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
func (service *RavenService) CreateCard(input *CreateCardInput) (Card, error) {
	if service.store == nil {
		return Card{}, &ServiceError{Message: cardStorageIsUnavailable}
	}

	card, err := service.store.CreateCard(createCardInput(nonNilInput(input)))
	if err == nil {
		service.schedulesChanged()
	}

	if err != nil {
		return publicCard(&card), &ServiceError{Message: err.Error()}
	}

	return publicCard(&card), nil
}

// UpdateCard updates card content while preserving retained item schedules.
func (service *RavenService) UpdateCard(input *UpdateCardInput) (Card, error) {
	if service.store == nil {
		return Card{}, &ServiceError{Message: cardStorageIsUnavailable}
	}

	card, err := service.store.UpdateCard(updateCardInput(nonNilInput(input)))
	if err == nil {
		service.schedulesChanged()
	}

	if err != nil {
		return publicCard(&card), &ServiceError{Message: err.Error()}
	}

	return publicCard(&card), nil
}

// ReviewItem records a rating and advances an item schedule.
func (service *RavenService) ReviewItem(
	folderID, cardID, itemID string,
	rating int,
) (Card, error) {
	if service.store == nil {
		return Card{}, &ServiceError{Message: cardStorageIsUnavailable}
	}

	card, err := service.store.ReviewItem(folderID, cardID, itemID, rating)
	if err == nil {
		service.schedulesChanged()
	}

	if err != nil {
		return publicCard(&card), &ServiceError{Message: err.Error()}
	}

	return publicCard(&card), nil
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
		service.app.Event.Emit(ravenSchedulesChanged)
	}

	service.RefreshReminders()
}

func main() {
	desktop := newDesktopComposition()
	desktop.prepare()

	err := desktop.run()
	if err != nil {
		desktop.service.app.Logger.ErrorContext(
			context.Background(),
			"application stopped",
			slog.Any(logErrorKey, err),
		)
		os.Exit(startupFailureExitCode)
	}
}

func newDesktopComposition() *desktopComposition {
	service := new(RavenService)

	service.notifier = notifications.New()

	return &desktopComposition{
		service:    service,
		visibility: autostart.NewVisibility(service.ShowMain),
		startup:    nil,
	}
}

func (desktop *desktopComposition) prepare() {
	desktop.prepareLoginDetection()
	desktop.createApplication()
	desktop.createWindows()
	desktop.configureServices()
	desktop.configureWidgetEvents()
	desktop.configureClosingHooks()
	desktop.configureLifecycleEvents()
	desktop.configureMenus()
}

func (desktop *desktopComposition) prepareLoginDetection() {
	loginnative.PrepareLoginDetection(func(nativeAutomatic bool) {
		// Cocoa callbacks must not wait for the UI thread.
		go desktop.visibility.Classify(
			nativeAutomatic || autostart.IsAutostartLaunch(os.Args[firstLaunchArgument:]),
		)
	})
}

func (desktop *desktopComposition) createApplication() {
	service := desktop.service
	app := application.New(desktop.applicationOptions())
	service.notifier.OnNotificationResponse(desktop.notificationResponse)

	service.app = app
	desktop.startup = autostart.New(startupwails.New(app.Autostart), app.Env.Info().Debug, nil)
}

func (desktop *desktopComposition) notificationResponse(result notifications.NotificationResult) {
	if result.Error != nil {
		return
	}

	if result.Response.ActionIdentifier == notifications.DefaultActionIdentifier {
		desktop.service.ShowReviews()
	}
}

func desktopIdentityOptions() application.Options {
	var options application.Options

	options.Name = appTitle
	options.Description = "A little practice. A little more remembered."
	options.Icon = appIcon

	return options
}

func (desktop *desktopComposition) applicationOptions() application.Options {
	options := desktopIdentityOptions()

	options.Services = []application.Service{
		application.NewService(
			&lifecycleAdapter{service: desktop.service, openStore: openServiceStore},
		),
		application.NewService(desktop.service),
	}
	options.Assets = desktop.assetOptions()
	options.Windows.DisableQuitOnLastWindowClosed = true
	options.Linux.DisableQuitOnLastWindowClosed = true
	options.SingleInstance = desktop.singleInstanceOptions()

	return options
}

func (desktop *desktopComposition) assetOptions() application.AssetOptions {
	var options application.AssetOptions

	options.Handler = application.AssetFileServerFS(assets)
	options.Middleware = desktop.service.mediaMiddleware

	return options
}

func (desktop *desktopComposition) singleInstanceOptions() *application.SingleInstanceOptions {
	var options application.SingleInstanceOptions

	options.UniqueID = autostart.ApplicationID
	options.OnSecondInstanceLaunch = func(data application.SecondInstanceData) {
		desktop.visibility.SecondInstance(data.Args)
	}

	return &options
}

func (desktop *desktopComposition) createWindows() {
	service := desktop.service

	service.mainWindow = service.app.Window.NewWithOptions(mainWindowOptions())
	service.widget = service.app.Window.NewWithOptions(widgetWindowOptions())
	service.reviewWindow = service.app.Window.NewWithOptions(reviewWindowOptions())
}

func mainWindowBounds() application.WebviewWindowOptions {
	var options application.WebviewWindowOptions

	options.Width = mainWindowWidth
	options.Height = mainWindowHeight
	options.MinWidth = mainWindowMinWidth
	options.MinHeight = mainWindowMinHeight

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

func (desktop *desktopComposition) configureServices() {
	service := desktop.service

	service.windows = companionwails.New(
		service.app,
		companionwails.WindowSet{
			Main:   service.mainWindow,
			Widget: service.widget,
			Review: service.reviewWindow,
		},
	)
	service.media = desktopmedia.New(
		service.app,
		service.mainWindow,
		func() http.Handler { return service.mediaHandler },
	)
}

func (desktop *desktopComposition) configureWidgetEvents() {
	service := desktop.service
	service.widget.OnWindowEvent(
		events.Common.WindowShow,
		func(_ *application.WindowEvent) { service.configureWidget() },
	)
	service.widget.OnWindowEvent(
		events.Common.WindowDidMove,
		func(_ *application.WindowEvent) { service.positionFloatingReview() },
	)
	service.widget.OnWindowEvent(
		events.Common.WindowDidResize,
		func(_ *application.WindowEvent) { service.configureWidget() },
	)
}

func (desktop *desktopComposition) configureClosingHooks() {
	service := desktop.service
	service.reviewWindow.RegisterHook(
		events.Common.WindowClosing,
		func(event *application.WindowEvent) { event.Cancel(); service.requestFloatingReviewClose() },
	)
	service.mainWindow.RegisterHook(
		events.Common.WindowClosing,
		func(event *application.WindowEvent) { event.Cancel(); service.HideMain() },
	)
	service.widget.RegisterHook(
		events.Common.WindowClosing,
		func(event *application.WindowEvent) { event.Cancel() },
	)
}

func (desktop *desktopComposition) configureLifecycleEvents() {
	app := desktop.service.app
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, desktop.applicationStarted)
	app.Event.OnApplicationEvent(
		events.Common.SystemDidWake,
		func(_ *application.ApplicationEvent) { desktop.service.RefreshReminders(); desktop.refreshStartup() },
	)
	app.Event.OnApplicationEvent(
		events.Mac.ApplicationShouldHandleReopen,
		func(_ *application.ApplicationEvent) { desktop.visibility.RequestOpen() },
	)
	app.Event.OnApplicationEvent(
		events.Mac.ApplicationDidBecomeActive,
		func(_ *application.ApplicationEvent) { desktop.refreshStartup() },
	)
	desktop.service.mainWindow.OnWindowEvent(
		events.Common.WindowFocus,
		func(_ *application.WindowEvent) { desktop.refreshStartup() },
	)
}

func (desktop *desktopComposition) applicationStarted(_ *application.ApplicationEvent) {
	service := desktop.service
	service.configureWidget()
	service.ResetWidget()
	service.startReminders()
	desktop.visibility.ApplicationStarted()
	desktop.reportStartupError(desktop.startup.Initialize(service.store))
}

func (desktop *desktopComposition) reportStartupError(err error) {
	if err == nil {
		return
	}

	app := desktop.service.app
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

func (desktop *desktopComposition) refreshStartup() {
	err := desktop.startup.Refresh()
	if err != nil {
		desktop.service.app.Logger.WarnContext(
			context.Background(),
			"could not refresh login startup status",
			slog.Any(logErrorKey, err),
		)
	}
}

func (desktop *desktopComposition) toggleStartup(_ *application.Context) {
	go func() { desktop.reportStartupError(desktop.startup.Toggle()) }()
}

func (desktop *desktopComposition) populateMenu(menu *application.Menu) *application.MenuItem {
	service := desktop.service

	menu.Add(openMenuTitle).OnClick(func(_ *application.Context) { service.ShowMain() })
	menu.Add(resetMenuTitle).OnClick(func(_ *application.Context) { service.ResetWidget() })
	menu.AddSeparator()

	startup := menu.AddCheckbox(startupMenuTitle, false).SetEnabled(false)
	startup.OnClick(desktop.toggleStartup)
	menu.AddSeparator()
	menu.Add(quitMenuTitle).OnClick(func(_ *application.Context) { service.Quit() })

	return startup
}

func (desktop *desktopComposition) configureMenus() {
	app := desktop.service.app
	menu := app.ContextMenu.New()
	widgetStartup := desktop.populateMenu(menu.Menu)
	app.ContextMenu.Add(appName, menu)

	trayMenu := app.Menu.New()
	trayStartup := desktop.populateMenu(trayMenu)
	tray := app.SystemTray.New().SetIcon(appIcon).SetMenu(trayMenu)
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

func (desktop *desktopComposition) run() error {
	err := desktop.service.app.Run()
	if err != nil {
		return fmt.Errorf("run desktop application: %w", err)
	}

	return nil
}

// Error returns the public service error message.
func (failure *ServiceError) Error() string { return failure.Message }

func mapValues[From, To any](values []From, convert func(*From) To) []To {
	if values == nil {
		return nil
	}

	result := make([]To, zero, len(values))
	for index := range values {
		result = append(result, convert(&values[index]))
	}

	return result
}
func publicFolder(value *library.Folder) Folder     { return Folder(*value) }
func publicFolders(value []library.Folder) []Folder { return mapValues(value, publicFolder) }
func publicReview(value *library.ItemReview) ItemReview {
	return ItemReview{Log: ReviewLog(value.Log), Automatic: value.Automatic}
}

func publicItem(value *library.ContentItem) ContentItem {
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

func publicCard(value *library.Card) Card {
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
	value *CreateFolderInput,
) library.CreateFolderInput {
	return library.CreateFolderInput(*value)
}
func itemInput(value *ItemDraft) library.ItemDraft { return library.ItemDraft(*value) }
func createCardInput(value *CreateCardInput) library.CreateCardInput {
	return library.CreateCardInput{
		FolderID: value.FolderID,
		Items:    mapValues(value.Items, itemInput),
	}
}

func updateCardInput(value *UpdateCardInput) library.UpdateCardInput {
	return library.UpdateCardInput{
		FolderID: value.FolderID,
		CardID:   value.CardID,
		Items:    mapValues(value.Items, itemInput),
	}
}

func publicMedia(value *library.SelectedMedia) SelectedMedia { return SelectedMedia(*value) }

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

func nonNilInput[Value any](value *Value) *Value {
	if value == nil {
		return new(Value)
	}

	return value
}
