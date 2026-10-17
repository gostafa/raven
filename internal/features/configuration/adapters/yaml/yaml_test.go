package yaml

import (
	"reflect"
	"strings"
	"testing"

	companion "raven/internal/features/companion/domain"
	library "raven/internal/shared/boltstore/domain"
)

func TestEmbeddedDefaultsPreserveExistingContracts(t *testing.T) {
	snapshot, err := (Loader{}).Load()
	if err != nil {
		t.Fatal(err)
	}
	want := values{
		ApplicationID:         "app.raven.desktop",
		LogErrorKey:           "error",
		LogOperationKey:       "operation",
		MediaRoutePrefix:      "/media/",
		PreviewRoutePrefix:    "/media-preview/",
		PreferencesStorageKey: "raven.preferences.v1",
		SchedulesChanged:      "raven:schedules-changed",
		ShowReviews:           "raven:show-reviews",
		OpenFloatingReview:    "raven:open-floating-review",
		CloseFloatingReview:   "raven:close-floating-review",
		Layout: companion.Layout{
			WidgetSize:   48,
			ReviewWidth:  420,
			ReviewHeight: 520,
			ReviewGap:    12,
		},
		Library: library.Rules{
			TextKind: "text", MinimumRating: 1, MaximumRating: 4,
			MIMEJPEG: "image/jpeg", MIMEOggAudio: "audio/ogg", MIMEMP4Video: "video/mp4",
		},
	}
	if !reflect.DeepEqual(snapshot.Settings, &settings{values: want}) {
		t.Fatalf("defaults changed: %+v", snapshot.Settings)
	}
}

func TestStrictYAMLAndRequiredValues(t *testing.T) {
	for name, app := range map[string]string{
		"malformed":        "layout: [",
		"duplicate":        appDefaults + "\npreferences_storage_key: duplicate\n",
		"unknown":          appDefaults + "\nunrecognized: value\n",
		"wrong type":       strings.Replace(appDefaults, "widget_size: 48", "widget_size: []", 1),
		"missing geometry": strings.Replace(appDefaults, "  widget_size: 48\n", "", 1),
		"missing key":      strings.Replace(appDefaults, "preferences_storage_key: raven.preferences.v1", "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if settings, err := read(
				strings.NewReader(app),
				strings.NewReader(sharedDefaults),
			); err == nil ||
				settings != nil {
				t.Fatalf("accepted invalid configuration: %+v, %v", settings, err)
			}
		})
	}
}

func TestAlternateSettingsReachTypedValues(t *testing.T) {
	app := strings.Replace(appDefaults, "review_width: 420", "review_width: 300", 1)
	app = strings.Replace(app, "raven.preferences.v1", "custom.preferences", 1)
	shared := strings.Replace(sharedDefaults, "raven:show-reviews", "custom:reviews", 1)
	settings, err := read(strings.NewReader(app), strings.NewReader(shared))
	if err != nil {
		t.Fatal(err)
	}
	if settings.Layout().ReviewWidth != 300 ||
		settings.PreferencesStorageKey() != "custom.preferences" ||
		settings.ShowReviews() != "custom:reviews" {
		t.Fatalf("settings not populated: %+v", settings)
	}
}

func TestLoadReportsEmbeddedParseFailure(t *testing.T) {
	previous := appDefaults
	appDefaults = "layout: ["
	t.Cleanup(func() { appDefaults = previous })
	if snapshot, err := (Loader{}).Load(); snapshot.Settings != nil || err == nil {
		t.Fatal("embedded parse failure was discarded")
	}
}

func TestLoadedSettingsReturnOwnedGeometryAndLibraryValues(t *testing.T) {
	snapshot, err := (Loader{}).Load()
	if err != nil {
		t.Fatal(err)
	}
	settings := snapshot.Settings
	layout, rules := settings.Layout(), settings.Library()
	wantLayout, wantRules := layout, rules
	layout.WidgetSize++
	rules.TextKind = "changed"
	if settings.Layout() != wantLayout || settings.Library() != wantRules {
		t.Fatal("settings exposed mutable configuration")
	}
}
