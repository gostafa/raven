package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	cardapi "raven/api/cards"
	configurationapi "raven/api/configuration"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	mediaapi "raven/api/media"
	companion "raven/internal/features/companion/domain"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
	desktopwails "raven/internal/features/desktop/adapters/wails"
	desktopports "raven/internal/features/desktop/ports"
	archive "raven/internal/features/folderarchive/domain"
	boltstore "raven/internal/shared/boltstore/adapters/bbolt"
	"raven/internal/shared/boltstore/ports"
	cffbatch "raven/internal/shared/cffbatch/adapters/cff"
	desktopmedia "raven/internal/shared/desktopmedia/adapters/wails"
	"raven/internal/shared/lifetime"
	localization "raven/internal/shared/localization/application"
	scheduling "raven/internal/shared/scheduling/adapters/fsrs"
)

func TestExecutableModuleResolvesFeaturePorts(t *testing.T) {
	if err := fx.ValidateApp(applicationModule, fx.Invoke(func(
		*RavenService,
		*desktopwails.Desktop,
		desktopports.Startup,
		desktopports.LaunchVisibility,
	) {
	})); err != nil {
		t.Fatal(err)
	}
}

type facadeArchives struct {
	result  archive.Result
	failure error
}

func (archives *facadeArchives) ExportFolders(string) (archive.Result, error) {
	return archives.result, archives.failure
}

func (archives *facadeArchives) ImportFolders(string) (archive.Result, error) {
	return archives.result, archives.failure
}

func TestFacadeFolderArchives(t *testing.T) {
	missing := newService(serviceParams{}, configurationFixture())
	if _, err := missing.ExportFolders(`{"folder_ids":["folder"]}`); err == nil {
		t.Fatal("missing exporter")
	}
	if _, err := missing.ImportFolders(archive.Fresh); err == nil {
		t.Fatal("missing importer")
	}
	archives := &facadeArchives{result: archive.Result{FolderCount: 1}}
	events := &facadeEvents{}
	reminders := &facadeReminders{}
	service := newService(
		serviceParams{Archives: archives, Events: events, Reminders: reminders},
		configurationFixture(),
	)
	if result, err := service.ExportFolders(
		`{"folder_ids":["folder"]}`,
	); err != nil ||
		!bytes.Contains([]byte(result), []byte(`"folder_count":1`)) {
		t.Fatal(result, err)
	}
	if _, err := service.ImportFolders(
		archive.Resume,
	); err != nil || len(events.names) != 1 ||
		reminders.wakes != 1 {
		t.Fatal(err, events.names)
	}
	archives.result.Canceled = true
	if _, err := service.ImportFolders(archive.Fresh); err != nil || len(events.names) != 1 {
		t.Fatal("canceled import published changes", err)
	}
	archives.failure = errors.New("internal archive failure")
	for _, operation := range []func() (string, error){func() (string, error) { return service.ExportFolders("{}") }, func() (string, error) { return service.ImportFolders(archive.Resume) }} {
		_, err := operation()
		var public *ServiceError
		if !errors.As(err, &public) || errors.Is(err, archives.failure) {
			t.Fatal("internal error escaped", err)
		}
	}
}

type desktopStub struct {
	failure    error
	registered int
}

func (desktop *desktopStub) Register(application.Service) { desktop.registered++ }
func (desktop *desktopStub) Run() error                   { return desktop.failure }

func TestExecutableGraphAndFailureReporting(t *testing.T) {
	module := applicationModule
	t.Cleanup(func() { applicationModule = module })
	desktop := &desktopStub{}
	runtime := lifetime.New()
	t.Cleanup(runtime.Cancel)
	applicationModule = fx.Options(fx.NopLogger, fx.Supply(
		fx.Annotate(desktop, fx.As(new(desktopRunner))),
		&RavenService{}, runtime,
	))
	main()
	if desktop.registered != 2 {
		t.Fatal("services were not registered")
	}
	failure := errors.New("desktop failed")
	desktop.failure = failure
	if err := run(); !errors.Is(err, failure) {
		t.Fatalf("lost desktop error: %v", err)
	}
	applicationModule = fx.Error(failure)
	if err := run(); !errors.Is(err, failure) {
		t.Fatalf("lost graph error: %v", err)
	}
	var output bytes.Buffer
	code := 0
	logger := slog.New(slog.NewTextHandler(&output, nil))
	exitOnError(failure, logger, func(value int) { code = value })
	if code != startupFailureExitCode || !bytes.Contains(output.Bytes(), []byte(failure.Error())) {
		t.Fatal("startup failure was not reported")
	}
	newFxLogger(logger).LogEvent(&fxevent.Started{})
	if desktopRuntime(&desktopwails.Desktop{}) == nil {
		t.Fatal("missing desktop runtime")
	}
}

type facadeWindows struct {
	companion.Windows
	calls []string
}

func (windows *facadeWindows) ShowMain() { windows.calls = append(windows.calls, "show") }

func (windows *facadeWindows) HideMain() { windows.calls = append(windows.calls, "hide") }

func (windows *facadeWindows) ResetWidget() { windows.calls = append(windows.calls, "reset") }

func (windows *facadeWindows) Quit() { windows.calls = append(windows.calls, "quit") }

func (windows *facadeWindows) ToggleFloatingReview() { windows.calls = append(windows.calls, "toggle") }

func (windows *facadeWindows) HideFloatingReview() {
	windows.calls = append(windows.calls, "hide-review")
}

type facadeEvents struct{ names []string }

func (events *facadeEvents) Emit(name string) { events.names = append(events.names, name) }

type facadeReminders struct{ wakes int }

func (*facadeReminders) Start()          {}
func (*facadeReminders) Stop()           {}
func (reminders *facadeReminders) Wake() { reminders.wakes++ }

type facadeMedia struct{ failure error }

func (media facadeMedia) Select(string) (string, error) {
	return encodeFacade([]mediaapi.Selection{{OriginalName: "test.png"}}), media.failure
}

type facadeRepository struct {
	ports.Repository
	failure error
}

func (repository facadeRepository) DeleteFolder(string) error       { return repository.failure }
func (repository facadeRepository) DeleteCard(string, string) error { return repository.failure }

func TestFacadeWindowCommandsAndDeletionEvents(t *testing.T) {
	windows, events, reminders := &facadeWindows{}, &facadeEvents{}, &facadeReminders{}
	deps := serviceParams{
		Windows:   windows,
		Events:    events,
		Reminders: reminders,
		Media:     facadeMedia{},
	}
	service := newService(deps, configurationFixture())
	service.ShowMain()
	service.HideMain()
	service.ResetWidget()
	service.Quit()
	service.ToggleFloatingReview()
	service.HideFloatingReview()
	service.ShowReviews()
	service.RefreshReminders()
	if !reflect.DeepEqual(
		windows.calls,
		[]string{"show", "hide", "reset", "quit", "toggle", "hide-review", "show"},
	) {
		t.Fatal(windows.calls)
	}
	if len(events.names) != 1 || events.names[0] != configurationFixture().ShowReviews() {
		t.Fatal(events.names)
	}
	for _, remove := range []func() error{func() error { return service.DeleteFolder("folder") }, func() error { return service.DeleteCard("folder", "card") }} {
		deps.Repository = nil
		service = newService(deps, configurationFixture())
		if err := remove(); err == nil {
			t.Fatal("missing storage accepted")
		}
		failure := errors.New("delete failed")
		deps.Repository = facadeRepository{failure: failure}
		service = newService(deps, configurationFixture())
		if err := remove(); err == nil || errors.Is(err, failure) {
			t.Fatalf("delete failure: %v", err)
		}
		deps.Repository = facadeRepository{}
		service = newService(deps, configurationFixture())
		if err := remove(); err != nil {
			t.Fatal(err)
		}
	}
	if reminders.wakes != 3 || len(events.names) != 3 {
		t.Fatal("deletions did not reconcile schedules")
	}
	selected, err := decodeFacade[[]mediaapi.Selection](service.SelectMediaFiles("image"))
	if err != nil || len(selected) != 1 || selected[0].OriginalName != "test.png" {
		t.Fatalf("media=%v error=%v", selected, err)
	}
}

func TestCompleteDependencyGraph(t *testing.T) {
	if err := fx.ValidateApp(applicationModule, fx.Invoke(func(*RavenService) {})); err != nil {
		t.Fatal(err)
	}
}

type failingRepository struct {
	ports.Repository
	failure error
}

func (r failingRepository) ListFolders() (string, error) { return "", r.failure }

func TestFacadeOwnsErrorContract(t *testing.T) {
	cause := fmt.Errorf("read library: %w", bolt.ErrDatabaseNotOpen)
	service := newService(
		serviceParams{Repository: failingRepository{failure: cause}},
		configurationFixture(),
	)
	_, err := decodeFacade[[]folderapi.Folder](service.ListFolders())
	var public *ServiceError
	if !errors.As(err, &public) || public.Message != cause.Error() {
		t.Fatalf("unexpected public error: %v", err)
	}
	if errors.Is(err, cause) || errors.Is(err, bolt.ErrDatabaseNotOpen) ||
		errors.Unwrap(err) != nil {
		t.Fatal("facade retained an implementation error chain")
	}
}

func TestFacadePreservesCompleteJSONContract(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 123000000, time.UTC)
	schedule, log, err := scheduling.New(new(configuration.Library(configurationFixture()))).
		Initialize(now)
	if err != nil {
		t.Fatal(err)
	}
	value := cardapi.Card{
		ID:        "card",
		FolderID:  "folder",
		CreatedAt: now.UnixMilli(),
		DueAt:     contentapi.Schedule(schedule).Due.UnixMilli(),
		Items: []contentapi.Item{
			{
				ID:              "item",
				FolderID:        "folder",
				CardID:          "card",
				Position:        0,
				Kind:            "image",
				OriginalName:    "diagram.png",
				MIMEType:        "image/png",
				ByteSize:        12,
				RelativePath:    "media/item.png",
				Schedule:        schedule,
				Reviews:         []contentapi.Review{{Log: log, Automatic: true}},
				LastNotifiedDue: contentapi.Schedule(schedule).Due.UnixMilli(),
			},
		},
	}
	for _, pair := range [][2]any{
		{value, facadeValue(t, value)},
		{folderapi.Folder{ID: "folder", Title: "Ideas", Tags: []string{}}, facadeValue(t, folderapi.Folder{ID: "folder", Title: "Ideas", Tags: []string{}})},
		{[]cardapi.Card(nil), facadeValue(t, []cardapi.Card(nil))},
	} {
		before, err := json.Marshal(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		after, err := json.Marshal(pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatalf("wire shape changed:\n%s\n%s", before, after)
		}
	}
}

func facadeValue[Value any](t *testing.T, value Value) Value {
	t.Helper()
	cause := errors.New("storage failure")
	response := publicResult(value, cause)
	result, err := response.value, response.failure
	var failure *ServiceError
	if !errors.As(err, &failure) || failure.Message != cause.Error() || errors.Is(err, cause) {
		t.Fatalf("public result leaked its implementation error: %v", err)
	}
	return result
}

func TestFolderServiceAndShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store, err := boltstore.Open(
		path,
		boltstore.NewDependencies(
			cffbatch.New(),
			scheduling.New(new(configuration.Library(configurationFixture()))),
			storeSettingsFixture(),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := newService(serviceParams{Repository: store}, configurationFixture())
	folder, err := decodeFacade[folderapi.Folder](
		service.CreateFolder(encodeFacade(&folderapi.CreateInput{Title: " Ideas "})),
	)
	if err != nil {
		t.Fatal(err)
	}
	folders, err := decodeFacade[[]folderapi.Folder](service.ListFolders())
	if err != nil || !reflect.DeepEqual(folders, []folderapi.Folder{folder}) {
		t.Fatalf("service folders: %+v, error=%v", folders, err)
	}
	card, err := decodeFacade[cardapi.Card](service.CreateCard(encodeFacade(
		&cardapi.CreateInput{
			FolderID: folder.ID,
			Items: []contentapi.Draft{
				{Kind: "text", Text: "A question"},
				{Kind: "text", Text: "An answer"},
			},
		},
	)))
	if err != nil {
		t.Fatal(err)
	}
	card, err = decodeFacade[cardapi.Card](service.ReviewItem(encodeFacade(
		&cardapi.ReviewInput{
			FolderID: folder.ID,
			CardID:   card.ID,
			ItemID:   contentapi.Items(card.Items)[0].ID,
			Rating:   3,
		},
	)))
	if err != nil || contentapi.Schedule(contentapi.Items(card.Items)[0].Schedule).Reps != 2 ||
		card.DueAt <= card.LastReviewed {
		t.Fatalf("service review: %+v, error=%v", card, err)
	}
	due := card.DueAt
	card, err = decodeFacade[cardapi.Card](service.UpdateCard(encodeFacade(
		&cardapi.UpdateInput{FolderID: folder.ID, CardID: card.ID, Items: []contentapi.Draft{
			{ID: contentapi.Items(card.Items)[0].ID, Kind: "text", Text: "An edited question"},
			{ID: contentapi.Items(card.Items)[1].ID, Kind: "text", Text: "An edited answer"},
		}},
	)))
	if err != nil || card.DueAt != due ||
		contentapi.Items(card.Items)[0].Text != "An edited question" {
		t.Fatalf("service edit: %+v, error=%v", card, err)
	}
	cards, err := decodeFacade[[]cardapi.Card](service.ListCards())
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card}) {
		t.Fatalf("service cards: %+v, error=%v", cards, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeFacade[[]folderapi.Folder](
		service.ListFolders(),
	); err == nil ||
		errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("shutdown did not close storage: %v", err)
	}
	reopened, err := boltstore.Open(
		path,
		boltstore.NewDependencies(
			cffbatch.New(),
			scheduling.New(new(configuration.Library(configurationFixture()))),
			storeSettingsFixture(),
		),
	)
	if err != nil {
		t.Fatalf("shutdown did not release the database lock: %v", err)
	}
	defer reopened.Close()
	rawFolders, err := decodeFacade[[]folderapi.Folder](reopened.ListFolders())
	folders = rawFolders
	if err != nil || !reflect.DeepEqual(folders, []folderapi.Folder{folder}) {
		t.Fatalf("folders after service shutdown: %+v, error=%v", folders, err)
	}
	rawCards, err := decodeFacade[[]cardapi.Card](reopened.ListCards())
	cards = rawCards
	if err != nil || !reflect.DeepEqual(cards, []cardapi.Card{card}) {
		t.Fatalf("cards after service shutdown: %+v, error=%v", cards, err)
	}
}

func TestMediaMiddlewareAndPickerErrors(t *testing.T) {
	service := newService(serviceParams{}, configurationFixture())
	next := http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) },
	)
	for _, test := range []struct {
		path   string
		status int
	}{{"/", http.StatusAccepted}, {"/media/folder/card/item", http.StatusServiceUnavailable}} {
		w := httptest.NewRecorder()
		desktopmedia.NewRegistry(cffbatch.New(), &desktopmedia.Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, mediaSettingsFixture()).
			Middleware(next).
			ServeHTTP(w, httptest.NewRequest("GET", test.path, nil))
		if w.Code != test.status {
			t.Fatalf("%s: got %d", test.path, w.Code)
		}
	}
	for _, kind := range []string{"image", "audio", "video", "invalid"} {
		if _, err := decodeFacade[[]mediaapi.Selection](
			service.SelectMediaFiles(kind),
		); err == nil {
			t.Fatalf("picker with unavailable window accepted %s", kind)
		}
	}
}

func TestFolderServiceWithoutStorage(t *testing.T) {
	service := newService(serviceParams{}, configurationFixture())
	if _, err := decodeFacade[[]folderapi.Folder](service.ListFolders()); err == nil {
		t.Fatal("listing without storage should fail")
	}
	if _, err := decodeFacade[folderapi.Folder](
		service.CreateFolder(encodeFacade(&folderapi.CreateInput{Title: "Ideas"})),
	); err == nil {
		t.Fatal("creating without storage should fail")
	}
	if _, err := decodeFacade[[]cardapi.Card](service.ListCards()); err == nil {
		t.Fatal("listing cards without storage should fail")
	}
	if _, err := decodeFacade[cardapi.Card](
		service.CreateCard(encodeFacade(&cardapi.CreateInput{})),
	); err == nil {
		t.Fatal("creating cards without storage should fail")
	}
	if _, err := decodeFacade[cardapi.Card](
		service.UpdateCard(encodeFacade(&cardapi.UpdateInput{})),
	); err == nil {
		t.Fatal("editing without storage should fail")
	}
	if _, err := decodeFacade[cardapi.Card](service.ReviewItem(encodeFacade(
		&cardapi.ReviewInput{FolderID: "folder", CardID: "card", ItemID: "item", Rating: 3},
	))); err == nil {
		t.Fatal("reviewing cards without storage should fail")
	}
}

func TestNullFacadeInputsUseExistingValidation(t *testing.T) {
	store, err := boltstore.Open(
		filepath.Join(t.TempDir(), "raven.db"),
		boltstore.NewDependencies(
			cffbatch.New(),
			scheduling.New(new(configuration.Library(configurationFixture()))),
			storeSettingsFixture(),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	service := newService(serviceParams{Repository: store}, configurationFixture())
	if _, err := decodeFacade[folderapi.Folder](
		service.CreateFolder(encodeFacade(nil)),
	); err == nil {
		t.Fatal("accepted empty folder input")
	}
	if _, err := decodeFacade[cardapi.Card](service.CreateCard(encodeFacade(nil))); err == nil {
		t.Fatal("accepted empty card input")
	}
	if _, err := decodeFacade[cardapi.Card](service.UpdateCard(encodeFacade(nil))); err == nil {
		t.Fatal("accepted empty card update")
	}
	folders, err := decodeFacade[[]folderapi.Folder](service.ListFolders())
	if err != nil || len(folders) != 0 {
		t.Fatalf("invalid inputs wrote records: %+v %v", folders, err)
	}
}

func configurationFixture() configuration.Settings {
	settings, err := configyaml.Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func storeSettingsFixture() *boltstore.Settings {
	settings := configurationFixture()
	return &boltstore.Settings{
		Rules:            configuration.Library(settings),
		MediaRoutePrefix: settings.MediaRoutePrefix(),
		LogErrorKey:      settings.LogErrorKey(),
	}
}

func mediaSettingsFixture() *desktopmedia.Settings {
	settings := configurationFixture()
	return &desktopmedia.Settings{
		MediaRoutePrefix:   settings.MediaRoutePrefix(),
		PreviewRoutePrefix: settings.PreviewRoutePrefix(),
		LogErrorKey:        settings.LogErrorKey(),
	}
}

func TestConfigurationFacadeReturnsOwnedFrontendSnapshot(t *testing.T) {
	settings := customConfiguration{Settings: configurationFixture()}
	service := newService(serviceParams{}, settings)
	value := mustFacade[configurationapi.Settings](service.GetConfiguration())
	if value.PreferencesStorageKey != settings.PreferencesStorageKey() ||
		value.MediaRoutePrefix != settings.MediaRoutePrefix() ||
		value.TextKind != configuration.Library(settings).TextKind ||
		value.SchedulesChanged != settings.SchedulesChanged() ||
		value.ShowReviews != settings.ShowReviews() ||
		value.OpenFloatingReview != settings.OpenFloatingReview() ||
		value.CloseFloatingReview != settings.CloseFloatingReview() {
		t.Fatal("configuration facade omitted or changed values", value)
	}
	value.PreferencesStorageKey = "edited"
	if mustFacade[configurationapi.Settings](
		service.GetConfiguration(),
	).PreferencesStorageKey != "custom.preferences" {
		t.Fatal("facade returned shared mutable state")
	}
}

type customConfiguration struct{ configuration.Settings }

func (customConfiguration) PreferencesStorageKey() string { return "custom.preferences" }
func (customConfiguration) MediaRoutePrefix() string      { return "/saved/" }

func encodeFacade(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func decodeFacade[Value any](payload string, err error) (Value, error) {
	var value Value
	if err != nil {
		return value, err
	}
	if err = json.Unmarshal([]byte(payload), &value); err != nil {
		return value, err
	}
	switch record := any(&value).(type) {
	case *cardapi.Card:
		err = cardapi.Normalize(record)
	case *[]cardapi.Card:
		for index := range *record {
			if err = cardapi.Normalize(&(*record)[index]); err != nil {
				break
			}
		}
	}
	return value, err
}

func mustFacade[Value any](payload string, err error) Value {
	value, err := decodeFacade[Value](payload, err)
	if err != nil {
		panic(err)
	}
	return value
}

func TestJSONEncodingFailureUsesPublicErrorContract(t *testing.T) {
	result := encodePublic(func() {})
	var failure *ServiceError
	if !errors.As(result.failure, &failure) || failure.Message == "" ||
		errors.Unwrap(result.failure) != nil {
		t.Fatalf("invalid transport error: %v", result.failure)
	}
}

type folderUpdateRepository struct {
	ports.Repository
	input   string
	failure error
}

func (repository *folderUpdateRepository) UpdateFolder(input string) (string, error) {
	repository.input = input
	return `{"id":"folder","title":"Updated","description":"","tags":[]}`, repository.failure
}

func TestFacadeUpdatesFolderAndTranslatesErrors(t *testing.T) {
	repository := &folderUpdateRepository{}
	service := newService(serviceParams{Repository: repository}, configurationFixture())
	input := `{"id":"folder","title":"Updated"}`
	folder, err := decodeFacade[folderapi.Folder](service.UpdateFolder(input))
	if err != nil || folder.ID != "folder" || folder.Title != "Updated" ||
		repository.input != input {
		t.Fatalf("folder update: %+v %v input=%s", folder, err, repository.input)
	}
	cause := fmt.Errorf("update failed: %w", bolt.ErrDatabaseNotOpen)
	repository.failure = cause
	_, err = service.UpdateFolder(input)
	var public *ServiceError
	if !errors.As(err, &public) || public.Message != cause.Error() || errors.Unwrap(err) != nil {
		t.Fatalf("folder update leaked implementation error: %v", err)
	}
	service = newService(serviceParams{}, configurationFixture())
	if _, err := service.UpdateFolder(input); err == nil {
		t.Fatal("updated folder without storage")
	}
}

type languagePreferenceFixture struct {
	value   string
	failure error
}

func (fixture *languagePreferenceFixture) LoadLanguagePreference() (string, error) {
	return fixture.value, nil
}

func (fixture *languagePreferenceFixture) SaveLanguagePreference(value string) error {
	if fixture.failure != nil {
		return fixture.failure
	}
	fixture.value = value
	return nil
}

type systemLanguageFixture struct{}

func (systemLanguageFixture) Locales() ([]string, error) { return []string{"en-US"}, nil }

func TestLocalizationFacadeAndSafeErrors(t *testing.T) {
	preferences := &languagePreferenceFixture{value: "system"}
	var warnings []error
	languages := localization.New(
		preferences,
		systemLanguageFixture{},
		func(err error) { warnings = append(warnings, err) },
	)
	languages.Initialize()
	operations := make(map[string]func(string, string) serviceResult[string])
	bindLocalizationOperations(operations, languages)
	service := &RavenService{operations: operations}
	state, err := service.GetLocalization()
	if err != nil || !strings.Contains(state, `"locale":"en"`) {
		t.Fatal(state, err)
	}
	state, err = service.SetLanguagePreference("ar")
	if err != nil || !strings.Contains(state, `"direction":"rtl"`) {
		t.Fatal(state, err)
	}
	if _, err = service.SetLanguagePreference("unsupported"); err == nil {
		t.Fatal("accepted invalid language")
	}
	preferences.failure = errors.New("private disk details")
	if _, err = service.SetLanguagePreference("en"); err == nil {
		t.Fatal("ignored persistence failure")
	}
	if len(warnings) != 1 || !errors.Is(warnings[0], preferences.failure) {
		t.Fatal("missing private diagnostic", warnings)
	}
	for _, failure := range []error{errors.New("private"), &ServiceError{Message: "private"}, &ServiceError{Code: "language_save_failed", Message: "private"}} {
		payload := string(marshalServiceError(failure))
		if strings.Contains(payload, "private") || !json.Valid([]byte(payload)) {
			t.Fatal(payload)
		}
	}
	if serviceOptions().MarshalError == nil {
		t.Fatal("missing error serializer")
	}
}
