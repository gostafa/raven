// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/multierr"

	configurationapi "raven/api/configuration"
	localizationapi "raven/api/localization"
	configuration "raven/internal/features/configuration/domain"
	desktopwails "raven/internal/features/desktop/adapters/wails"
	desktopcomposition "raven/internal/features/desktop/composition"
	desktopports "raven/internal/features/desktop/ports"
	archive "raven/internal/features/folderarchive/domain"
	libraryports "raven/internal/shared/boltstore/ports"
	localization "raven/internal/shared/localization/application"
	localizationdomain "raven/internal/shared/localization/domain"
)

func newService(deps serviceParams, settings configuration.Settings) *RavenService {
	operations := libraryOperations(deps.Repository)
	bindLocalizationOperations(operations, deps.Localization)

	changed := func() { schedulesChanged(deps, settings) }
	bindCardOperations(operations, deps.Repository, changed)
	bindFolderMutations(operations, deps.Repository)
	bindWindowOperations(operations, deps)
	bindAuxiliaryOperations(operations, deps, settings)
	bindArchiveOperations(operations, deps, changed)

	return &RavenService{operations: operations}
}

func bindNotificationOperations(
	operations map[string]func(string, string) serviceResult[string],
	notifications desktopports.Notifications,
) {
	operations["Notify"] = func(title, body string) serviceResult[string] {
		if notifications == nil {
			return serviceResult[string]{value: "", failure: nil}
		}

		return publicResult("", notifications.Notify(title, body))
	}
}

// Notify displays an application message through native OS notifications.
func (service *RavenService) Notify(title, body string) error {
	return service.operations["Notify"](title, body).failure
}

func libraryOperations(
	repository libraryports.Repository,
) map[string]func(string, string) serviceResult[string] {
	return map[string]func(string, string) serviceResult[string]{
		"ListFolders": func(string, string) serviceResult[string] {
			return readLibrary(
				repository,
				folderStorageIsUnavailable,
				libraryports.Repository.ListFolders,
			)
		},
		"ListCards": func(string, string) serviceResult[string] {
			return readLibrary(
				repository,
				cardStorageIsUnavailable,
				libraryports.Repository.ListCards,
			)
		},
	}
}

func bindFolderMutations(
	operations map[string]func(string, string) serviceResult[string],
	repository libraryports.Repository,
) {
	operations["UpdateFolder"] = func(input, _ string) serviceResult[string] {
		return editLibrary(repository, input, libraryports.Repository.UpdateFolder)
	}
	operations["CreateFolder"] = func(input, _ string) serviceResult[string] { return createFolder(repository, input) }
}

func createFolder(repository libraryports.Repository, input string) serviceResult[string] {
	if repository == nil {
		return publicResult(
			"",
			&ServiceError{Message: folderStorageIsUnavailable, Code: actionFailedCode},
		)
	}

	return publicResult(repository.CreateFolder(input))
}

func bindCardOperations(
	operations map[string]func(string, string) serviceResult[string],
	repository libraryports.Repository,
	changed func(),
) {
	edits := map[string]func(libraryports.Repository, string) (string, error){
		"CreateCard": libraryports.Repository.CreateCard,
		"UpdateCard": libraryports.Repository.UpdateCard,
		"ReviewItem": libraryports.Repository.ReviewItem,
	}

	for name := range edits {
		edit := edits[name]

		operations[name] = func(input, _ string) serviceResult[string] {
			return changedResult(editLibrary(repository, input, edit), changed)
		}
	}

	bindDeletionOperations(operations, repository, changed)
}

func editLibrary(
	repository libraryports.Repository,
	input string,
	edit func(libraryports.Repository, string) (string, error),
) serviceResult[string] {
	if repository == nil {
		return publicResult(
			"",
			&ServiceError{Message: cardStorageIsUnavailable, Code: actionFailedCode},
		)
	}

	return publicResult(edit(repository, input))
}

func bindDeletionOperations(
	operations map[string]func(string, string) serviceResult[string],
	repository libraryports.Repository,
	changed func(),
) {
	operations["DeleteFolder"] = folderDeletion(repository, changed)
	operations["DeleteCard"] = cardDeletion(repository, changed)
}

func folderDeletion(
	repository libraryports.Repository,
	changed func(),
) func(string, string) serviceResult[string] {
	return func(folder, _ string) serviceResult[string] {
		result := deleteLibrary(
			repository,
			folderStorageIsUnavailable,
			func(repo libraryports.Repository) error {
				return errors.Join(repo.DeleteFolder(folder))
			},
		)

		return changedResult(result, changed)
	}
}

func cardDeletion(
	repository libraryports.Repository,
	changed func(),
) func(string, string) serviceResult[string] {
	return func(folder, card string) serviceResult[string] {
		result := deleteLibrary(
			repository,
			cardStorageIsUnavailable,
			func(repo libraryports.Repository) error {
				return errors.Join(repo.DeleteCard(folder, card))
			},
		)

		return changedResult(result, changed)
	}
}

func deleteLibrary(
	repository libraryports.Repository,
	unavailable string,
	remove func(libraryports.Repository) error,
) serviceResult[string] {
	if repository == nil {
		return publicResult("", &ServiceError{Message: unavailable, Code: actionFailedCode})
	}

	return publicResult("", remove(repository))
}

func changedResult(result serviceResult[string], changed func()) serviceResult[string] {
	if result.failure == nil {
		changed()
	}

	return result
}

func bindWindowOperations(
	operations map[string]func(string, string) serviceResult[string],
	deps serviceParams,
) {
	actions := windowActions(deps)

	for name := range actions {
		action := actions[name]

		operations[name] = func(string, string) serviceResult[string] {
			action()

			return serviceResult[string]{value: "", failure: nil}
		}
	}
}

func windowActions(deps serviceParams) map[string]func() {
	return map[string]func(){
		"ShowMain":             func() { deps.Windows.ShowMain() },
		"HideMain":             func() { deps.Windows.HideMain() },
		"ResetWidget":          func() { deps.Windows.ResetWidget() },
		"Quit":                 func() { deps.Windows.Quit() },
		"ToggleFloatingReview": func() { deps.Windows.ToggleFloatingReview() },
		"SyncFloatingReview":   func() { deps.Windows.SyncFloatingReview() },
		"HideFloatingReview":   func() { deps.Windows.HideFloatingReview() },
	}
}

func bindAuxiliaryOperations(
	operations map[string]func(string, string) serviceResult[string],
	deps serviceParams,
	settings configuration.Settings,
) {
	bindNotificationOperations(operations, deps.Notifications)
	bindTransferOperations(operations, deps, func() { schedulesChanged(deps, settings) })

	operations["GetConfiguration"] = func(string, string) serviceResult[string] {
		return encodePublic(configurationSnapshot(settings))
	}
	operations["SelectMediaFiles"] = func(kind, _ string) serviceResult[string] { return selectMedia(deps, kind) }
	bindInteractionOperations(operations, deps, settings)
}

func bindInteractionOperations(
	operations map[string]func(string, string) serviceResult[string],
	deps serviceParams,
	settings configuration.Settings,
) {
	operations["ShowReviews"] = func(string, string) serviceResult[string] {
		deps.Windows.ShowMain()
		deps.Events.Emit(settings.ShowReviews())

		return serviceResult[string]{value: "", failure: nil}
	}
	operations["RefreshReminders"] = func(string, string) serviceResult[string] {
		refreshReminders(deps)

		return serviceResult[string]{value: "", failure: nil}
	}
}

func schedulesChanged(deps serviceParams, settings configuration.Settings) {
	if deps.Events != nil {
		deps.Events.Emit(settings.SchedulesChanged())
	}

	refreshReminders(deps)
}

func refreshReminders(deps serviceParams) {
	if deps.Reminders != nil {
		deps.Reminders.Wake()
	}
}

func selectMedia(deps serviceParams, kind string) serviceResult[string] {
	if deps.Media == nil {
		return publicResult(
			"",
			&ServiceError{Message: "file selection is unavailable", Code: actionFailedCode},
		)
	}

	return publicResult(deps.Media.Select(kind))
}

func encodePublic(value any) serviceResult[string] {
	data, err := json.Marshal(value)

	return publicResult(string(data), err)
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
	desktop.Register(
		application.NewServiceWithOptions(
			service,
			serviceOptions(),
		),
	)

	err := multierr.Append(desktop.Run(), bridge.ServiceShutdown())
	if err != nil {
		return fmt.Errorf("run and shut down application: %w", err)
	}

	return nil
}

func desktopRuntime(desktop *desktopwails.Desktop) *desktopwails.Desktop { return desktop }

// ListFolders returns folders in creation order as JSON.
func (service *RavenService) ListFolders() (string, error) {
	result := service.operations["ListFolders"]("", "")

	return result.value, result.failure
}

// CreateFolder creates a folder from JSON metadata and returns its JSON record.
func (service *RavenService) CreateFolder(input string) (string, error) {
	result := service.operations["CreateFolder"](input, "")

	return result.value, result.failure
}

// ListCards returns persisted library cards as JSON.
func (service *RavenService) ListCards() (string, error) {
	result := service.operations["ListCards"]("", "")

	return result.value, result.failure
}

// DeleteFolder deletes a folder and its content.
func (service *RavenService) DeleteFolder(folderID string) error {
	return service.operations["DeleteFolder"](folderID, "").failure
}

// DeleteCard deletes a card from its owning folder.
func (service *RavenService) DeleteCard(folderID, cardID string) error {
	return service.operations["DeleteCard"](folderID, cardID).failure
}

// CreateCard creates a card from JSON and returns its initialized JSON record.
func (service *RavenService) CreateCard(input string) (string, error) {
	result := service.operations["CreateCard"](input, "")

	return result.value, result.failure
}

// UpdateCard replaces JSON card content while retaining existing schedules.
func (service *RavenService) UpdateCard(input string) (string, error) {
	result := service.operations["UpdateCard"](input, "")

	return result.value, result.failure
}

// ReviewItem records a JSON rating and returns the updated JSON card.
func (service *RavenService) ReviewItem(input string) (string, error) {
	result := service.operations["ReviewItem"](input, "")

	return result.value, result.failure
}

// ShowMain shows and focuses the main library window.
func (service *RavenService) ShowMain() { service.operations["ShowMain"]("", "") }

// HideMain hides the main library window.
func (service *RavenService) HideMain() { service.operations["HideMain"]("", "") }

// ResetWidget moves the floating widget onto the primary screen.
func (service *RavenService) ResetWidget() { service.operations["ResetWidget"]("", "") }

// Quit terminates Raven.
func (service *RavenService) Quit() { service.operations["Quit"]("", "") }

// ShowReviews opens the library review session.
func (service *RavenService) ShowReviews() { service.operations["ShowReviews"]("", "") }

// RefreshReminders requests reconciliation of persisted review schedules.
func (service *RavenService) RefreshReminders() { service.operations["RefreshReminders"]("", "") }

// ToggleFloatingReview opens or requests closing the floating review window.
func (service *RavenService) ToggleFloatingReview() {
	service.operations["ToggleFloatingReview"]("", "")
}

// HideFloatingReview hides the floating review window.
func (service *RavenService) HideFloatingReview() { service.operations["HideFloatingReview"]("", "") }

// SyncFloatingReview synchronizes an open review after the frontend subscribes.
func (service *RavenService) SyncFloatingReview() { service.operations["SyncFloatingReview"]("", "") }

// SelectMediaFiles returns selected draft media as JSON.
func (service *RavenService) SelectMediaFiles(kind string) (string, error) {
	result := service.operations["SelectMediaFiles"](kind, "")

	return result.value, result.failure
}

// GetConfiguration returns the frontend configuration snapshot as JSON.
func (service *RavenService) GetConfiguration() (string, error) {
	result := service.operations["GetConfiguration"]("", "")

	return result.value, result.failure
}

// Error returns the public service error message.
func (failure *ServiceError) Error() string { return failure.Message }

func publicResult[Value any](value Value, err error) serviceResult[Value] {
	result := serviceResult[Value]{value: value, failure: nil}

	if err != nil {
		result.failure = &ServiceError{
			Message: err.Error(),
			Code:    actionFailedCode,
		}
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

		return publicResult(empty, &ServiceError{Message: unavailable, Code: actionFailedCode})
	}

	return publicResult(read(repository))
}

func configurationSnapshot(settings configuration.Settings) configurationapi.Settings {
	return configurationapi.Settings{
		LocalizationChanged:   "raven:localization-changed",
		PreferencesStorageKey: settings.PreferencesStorageKey(),
		MediaRoutePrefix:      settings.MediaRoutePrefix(),
		TextKind:              configuration.Library(settings).TextKind,
		SchedulesChanged:      settings.SchedulesChanged(),
		ShowReviews:           settings.ShowReviews(),
		OpenFloatingReview:    settings.OpenFloatingReview(),
		CloseFloatingReview:   settings.CloseFloatingReview(),
	}
}

// UpdateFolder updates a folder's JSON metadata and returns its JSON record.
func (service *RavenService) UpdateFolder(input string) (string, error) {
	result := service.operations["UpdateFolder"](input, "")

	return result.value, result.failure
}

func bindArchiveOperations(
	operations map[string]func(string, string) serviceResult[string],
	deps serviceParams,
	changed func(),
) {
	operations["ExportFolders"] = func(input, _ string) serviceResult[string] {
		if deps.Archives == nil {
			return publicResult(
				"",
				&ServiceError{Message: folderArchivesUnavailable, Code: actionFailedCode},
			)
		}

		result, err := deps.Archives.ExportFolders(input)

		return archiveResult(&result, err)
	}
	operations["ImportFolders"] = func(mode, _ string) serviceResult[string] {
		return importArchive(deps, mode, changed)
	}
}

func importArchive(deps serviceParams, mode string, changed func()) serviceResult[string] {
	if deps.Archives == nil {
		return publicResult(
			"",
			&ServiceError{Message: folderArchivesUnavailable, Code: actionFailedCode},
		)
	}

	result, err := deps.Archives.ImportFolders(mode)
	if err == nil && !result.Canceled {
		changed()
	}

	return archiveResult(&result, err)
}

func archiveResult(result *archive.Result, err error) serviceResult[string] {
	if err != nil {
		return publicResult("", err)
	}

	return encodePublic(result)
}

// ExportFolders exports selected folders to a portable archive using a native save dialog.
func (service *RavenService) ExportFolders(input string) (string, error) {
	result := service.operations["ExportFolders"](input, "")

	return result.value, result.failure
}

// ImportFolders imports folder copies using fresh or resume review scheduling.
func (service *RavenService) ImportFolders(mode string) (string, error) {
	result := service.operations["ImportFolders"](mode, "")

	return result.value, result.failure
}

func bindLocalizationOperations(
	operations map[string]func(string, string) serviceResult[string],
	service *localization.Service,
) {
	operations["GetLocalization"] = func(string, string) serviceResult[string] {
		return encodePublic(localizationSnapshot(service.Current()))
	}
	operations["SetLanguagePreference"] = func(preference, _ string) serviceResult[string] {
		return setLanguagePreference(service, preference)
	}
}

func setLanguagePreference(service *localization.Service, preference string) serviceResult[string] {
	state, err := service.Set(preference)
	if err != nil {
		return serviceResult[string]{value: "", failure: languageFailure(err)}
	}

	return encodePublic(localizationSnapshot(state))
}

func languageFailure(err error) *ServiceError {
	code := "language_save_failed"

	if errors.Is(err, localization.ErrInvalidPreference) {
		code = "invalid_language"
	}

	return &ServiceError{Code: code, Message: "Language preference could not be updated."}
}

func localizationSnapshot(state localizationdomain.State) localizationapi.State {
	return localizationapi.State{
		Preference: state.Preference,
		Locale:     state.Locale,
		Direction:  state.Direction,
		Revision:   state.Revision,
	}
}

// GetLocalization returns the saved language and its resolved presentation as JSON.
func (service *RavenService) GetLocalization() (string, error) {
	result := service.operations["GetLocalization"]("", "")

	return result.value, result.failure
}

// SetLanguagePreference persists system, en, or ar before publishing a language change.
func (service *RavenService) SetLanguagePreference(preference string) (string, error) {
	result := service.operations["SetLanguagePreference"](preference, "")

	return result.value, result.failure
}

func marshalServiceError(err error) []byte {
	code := actionFailedCode

	var failure *ServiceError

	if errors.As(err, &failure) && failure.Code != "" {
		code = failure.Code
	}

	return []byte(
		`{"code":` + strconv.Quote(
			code,
		) + `,"message":"Raven could not complete that action. Please try again."}`,
	)
}

func serviceOptions() application.ServiceOptions {
	var options application.ServiceOptions

	options.MarshalError = marshalServiceError

	return options
}

func bindTransferOperations(
	operations map[string]func(string, string) serviceResult[string],
	deps serviceParams,
	changed func(),
) {
	operations["TransferCards"] = func(input, _ string) serviceResult[string] {
		if deps.Transfers == nil {
			return publicResult(
				"",
				&ServiceError{Message: cardStorageIsUnavailable, Code: actionFailedCode},
			)
		}

		return changedResult(publicResult(deps.Transfers.TransferCards(input)), changed)
	}
}

// TransferCards atomically copies or moves cards described by JSON.
func (service *RavenService) TransferCards(input string) (string, error) {
	result := service.operations["TransferCards"](input, "")

	return result.value, result.failure
}
