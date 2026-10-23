// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	api "raven/api/localization"
	desktop "raven/internal/features/desktop/ports"
	localization "raven/internal/shared/localization/application"
	"raven/internal/shared/localization/ports"
)

type preferencesFixture struct {
	preference string
	failure    error
}

func (preferences *preferencesFixture) LoadLanguagePreference() (string, error) {
	return preferences.preference, preferences.failure
}

func (preferences *preferencesFixture) SaveLanguagePreference(value string) error {
	preferences.preference = value
	return nil
}

type systemFixture struct{}

func (systemFixture) Locales() ([]string, error) { return []string{"ar-EG"}, nil }

func TestModuleInitializesBeforePublishingAndReleasesObservers(t *testing.T) {
	var output bytes.Buffer
	app := application.New(application.Options{})
	preferences := &preferencesFixture{
		preference: "system",
		failure:    errors.New("read unavailable"),
	}
	var service *localization.Service
	var messages desktop.Localization
	graph := fxtest.New(
		t,
		Module,
		fx.Supply(
			app,
			slog.New(slog.NewTextHandler(&output, nil)),
			fx.Annotate(preferences, fx.As(new(ports.Preferences))),
		),
		fx.Replace(fx.Annotate(systemFixture{}, fx.As(new(ports.SystemLocales)))),
		fx.Populate(&service, &messages),
	)
	received := make(chan api.State, 4)
	cancel := app.Event.On(
		"raven:localization-changed",
		func(event *application.CustomEvent) { received <- event.Data.(api.State) },
	)
	defer cancel()
	graph.RequireStart()
	if service.Current().Locale != "ar" || messages.Text("open", 0) != "فتح Raven" {
		t.Fatal(service.Current())
	}
	if !strings.Contains(output.String(), "read unavailable") {
		t.Fatal(output.String())
	}
	select {
	case state := <-received:
		if state.Locale != "ar" {
			t.Fatal(state)
		}
	case <-time.After(time.Second):
		t.Fatal("missing initial event")
	}
	graph.RequireStop()
	if _, err := service.Set("en"); err != nil {
		t.Fatal(err)
	}
	select {
	case state := <-received:
		t.Fatal("observer retained after shutdown", state)
	case <-time.After(10 * time.Millisecond):
	}
}

func TestModuleRegistersTheSystemAdapter(t *testing.T) {
	var system ports.SystemLocales
	graph := fxtest.New(
		t,
		Module,
		fx.Supply(
			application.New(application.Options{}),
			slog.Default(),
			fx.Annotate(&preferencesFixture{preference: "en"}, fx.As(new(ports.Preferences))),
		),
		fx.Populate(&system),
	)
	if system == nil {
		t.Fatal("missing OS adapter")
	}
	graph.RequireStart().RequireStop()
}
