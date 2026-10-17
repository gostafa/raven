// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/multierr"

	cardapi "raven/api/cards"
	configurationapi "raven/api/configuration"
	folderapi "raven/api/folders"
	mediaapi "raven/api/media"
	configuration "raven/internal/features/configuration/domain"
	desktopwails "raven/internal/features/desktop/adapters/wails"
	desktopcomposition "raven/internal/features/desktop/composition"
	libraryports "raven/internal/shared/boltstore/ports"
)

func newService(deps serviceParams, settings configuration.Settings) *RavenService {
	return &RavenService{
		configuration: settings,
		store:         deps.Repository,
		media:         deps.Media,
		windows:       deps.Windows,
		events:        deps.Events,
		reminders:     deps.Reminders,
	}
}

func newLogger() *slog.Logger { return slog.New(slog.NewTextHandler(os.Stderr, nil)) }

//nolint:ireturn // Fx.WithLogger requires a constructor returning its event logger interface.
func newFxLogger(logger *slog.Logger) fxevent.Logger {
	var events fxevent.SlogLogger

	events.Logger = logger
	events.UseLogLevel(slog.LevelDebug)

	return &events
}

func main() {
	exitOnError(run(), newLogger(), os.Exit)
}

func exitOnError(err error, logger *slog.Logger, exit func(int)) {
	if err != nil {
		logger.ErrorContext(
			context.Background(),
			"application stopped",
			slog.String(startupDetailsKey, err.Error()),
		)
		exit(startupFailureExitCode)
	}
}

func run() error {
	var services applicationParams

	graph, err := newApplicationGraph(&services)
	if err != nil {
		return fmt.Errorf("construct application graph: %w", err)
	}

	bridge := desktopcomposition.NewBridge(graph, services.Runtime)

	err = runDesktop(services.Desktop, bridge, services.Service)
	if err != nil {
		return fmt.Errorf("run desktop: %w", err)
	}

	return nil
}

func newApplicationGraph(services *applicationParams) (*fx.App, error) {
	graph := fx.New(applicationModule, fx.Populate(services))

	err := graph.Err()
	if err != nil {
		return nil, fmt.Errorf("populate application services: %w", err)
	}

	return graph, nil
}

func runDesktop(
	desktop desktopRunner,
	bridge *desktopcomposition.Bridge,
	service *RavenService,
) error {
	desktop.Register(application.NewService(bridge))
	desktop.Register(application.NewService(service))

	err := multierr.Append(desktop.Run(), bridge.ServiceShutdown())
	if err != nil {
		return fmt.Errorf("run and shut down application: %w", err)
	}

	return nil
}

func desktopRuntime(desktop *desktopwails.Desktop) *desktopwails.Desktop { return desktop }

// ListFolders returns folders in creation order.
func (service *RavenService) ListFolders() ([]folderapi.Folder, error) {
	result := readLibrary(
		service.store,
		folderStorageIsUnavailable,
		libraryports.Repository.ListFolders,
	)

	return result.value, result.failure
}

// CreateFolder creates a folder from editable metadata.
func (service *RavenService) CreateFolder(input *folderapi.CreateInput) (folderapi.Folder, error) {
	if service.store == nil {
		return folderapi.Folder{}, &ServiceError{Message: folderStorageIsUnavailable}
	}

	result := publicResult(service.store.CreateFolder(nonNilInput(input)))

	return result.value, result.failure
}

// ListCards returns the persisted library cards.
func (service *RavenService) ListCards() ([]cardapi.Card, error) {
	result := readLibrary(
		service.store,
		cardStorageIsUnavailable,
		libraryports.Repository.ListCards,
	)

	return result.value, result.failure
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
func (service *RavenService) CreateCard(input *cardapi.CreateInput) (cardapi.Card, error) {
	result := changeCard(service, input, libraryports.Cards.CreateCard)

	return result.value, result.failure
}

// UpdateCard updates card content while preserving retained item schedules.
func (service *RavenService) UpdateCard(input *cardapi.UpdateInput) (cardapi.Card, error) {
	result := changeCard(service, input, libraryports.Cards.UpdateCard)

	return result.value, result.failure
}

// ReviewItem records a rating and advances an item schedule.
func (service *RavenService) ReviewItem(input *cardapi.ReviewInput) (cardapi.Card, error) {
	result := changeCard(service, input, libraryports.Cards.ReviewItem)

	return result.value, result.failure
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
	service.events.Emit(service.configuration.ShowReviews())
}

// RefreshReminders requests reconciliation of persisted review schedules.
func (service *RavenService) RefreshReminders() {
	if service.reminders != nil {
		service.reminders.Wake()
	}
}

func (service *RavenService) schedulesChanged() {
	if service.events != nil {
		service.events.Emit(service.configuration.SchedulesChanged())
	}

	service.RefreshReminders()
}

// Error returns the public service error message.
func (failure *ServiceError) Error() string { return failure.Message }

func publicResult[Value any](value Value, err error) serviceResult[Value] {
	result := serviceResult[Value]{value: value, failure: nil}

	if err != nil {
		result.failure = &ServiceError{Message: err.Error()}
	}

	return result
}

func readLibrary[Value any](
	repository libraryports.Repository,
	unavailable string,
	read func(libraryports.Repository) (Value, error),
) serviceResult[Value] {
	if repository == nil {
		var empty Value

		return publicResult(empty, &ServiceError{Message: unavailable})
	}

	return publicResult(read(repository))
}

func changeCard[Input any](
	service *RavenService,
	input *Input,
	change func(libraryports.Cards, *Input) (cardapi.Card, error),
) serviceResult[cardapi.Card] {
	var card cardapi.Card

	if service.store == nil {
		return publicResult(card, &ServiceError{Message: cardStorageIsUnavailable})
	}

	card, err := change(service.store, nonNilInput(input))
	if err == nil {
		service.schedulesChanged()
	}

	return publicResult(card, err)
}

// SelectMediaFiles selects local media for a card draft.
func (service *RavenService) SelectMediaFiles(kind string) ([]mediaapi.Selection, error) {
	if service.media == nil {
		return nil, &ServiceError{Message: "file selection is unavailable"}
	}

	result := publicResult(service.media.Select(kind))

	return result.value, result.failure
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

// GetConfiguration returns the frontend's immutable configuration snapshot.
func (service *RavenService) GetConfiguration() configurationapi.Settings {
	settings := service.configuration

	return configurationapi.Settings{
		PreferencesStorageKey: settings.PreferencesStorageKey(),
		MediaRoutePrefix:      settings.MediaRoutePrefix(),
		TextKind:              configuration.Library(settings).TextKind,
		SchedulesChanged:      settings.SchedulesChanged(),
		ShowReviews:           settings.ShowReviews(),
		OpenFloatingReview:    settings.OpenFloatingReview(),
		CloseFloatingReview:   settings.CloseFloatingReview(),
	}
}
