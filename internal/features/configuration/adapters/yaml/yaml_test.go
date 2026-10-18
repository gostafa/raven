package yaml

import (
	"reflect"
	"strings"
	"testing"

	companion "raven/internal/features/companion/domain"
	configuration "raven/internal/features/configuration/domain"
	library "raven/internal/shared/boltstore/domain"
)

func TestEmbeddedDefaultsPreserveExistingContracts(t *testing.T) {
	snapshot, err := Load()
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
	want.Library, want.Layout = encodeContract(want.Library), encodeContract(want.Layout)
	if !reflect.DeepEqual(snapshot, &Settings{values: want}) {
		t.Fatalf("defaults changed: %+v", snapshot)
	}
}

func TestStrictYAMLAndRequiredValues(t *testing.T) {
	for name, app := range map[string]string{
		"malformed":  "layout: [",
		"duplicate":  appDefaults + "\npreferences_storage_key: duplicate\n",
		"unknown":    appDefaults + "\nunrecognized: value\n",
		"wrong type": strings.Replace(appDefaults, "widget_size: 48", "widget_size: []", 1),
		"missing geometry": strings.ReplaceAll(appDefaults,
			"  widget_size: 48\n  review_width: 420\n  review_height: 520\n  review_gap: 12\n", ""),
		"missing key": strings.Replace(appDefaults, "preferences_storage_key: raven.preferences.v1", "", 1),
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
	if configuration.Layout(settings).ReviewWidth != 300 ||
		settings.PreferencesStorageKey() != "custom.preferences" ||
		settings.ShowReviews() != "custom:reviews" {
		t.Fatalf("settings not populated: %+v", settings)
	}
}

func TestLoadReportsEmbeddedParseFailure(t *testing.T) {
	previous := appDefaults
	appDefaults = "layout: ["
	t.Cleanup(func() { appDefaults = previous })
	if snapshot, err := Load(); snapshot != nil || err == nil {
		t.Fatal("embedded parse failure was discarded")
	}
}

func TestLoadedSettingsReturnOwnedGeometryAndLibraryValues(t *testing.T) {
	snapshot, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	settings := snapshot
	layout, rules := configuration.Layout(settings), configuration.Library(settings)
	wantLayout, wantRules := layout, rules
	layout.WidgetSize++
	rules.TextKind = "changed"
	if configuration.Layout(settings) != wantLayout ||
		configuration.Library(settings) != wantRules {
		t.Fatal("settings exposed mutable configuration")
	}
}

func TestNestedContractsRejectUnknownAndInvalidValues(t *testing.T) {
	for name, shared := range map[string]string{
		"unknown library field": strings.Replace(sharedDefaults, "  text_kind: text", "  text_kind: text\n  unknown: value", 1),
		"invalid library type":  strings.Replace(sharedDefaults, "  minimum_rating: 1", "  minimum_rating: []", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if settings, err := read(
				strings.NewReader(appDefaults),
				strings.NewReader(shared),
			); err == nil ||
				settings != nil {
				t.Fatal("accepted invalid nested configuration", settings, err)
			}
		})
	}
}

func TestSerializedContractsAreOwnedAndRejectUnsupportedValues(t *testing.T) {
	settings, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	library, layout := settings.Library(), settings.Layout()
	library[0], layout[0] = '[', '['
	if configuration.Library(settings).TextKind != "text" ||
		configuration.Layout(settings).WidgetSize != 48 {
		t.Fatal("serialized values exposed mutable configuration")
	}
	if encodeContract(func() {}) != nil {
		t.Fatal("unsupported configuration produced JSON")
	}
	if empty := new(Settings); empty.Library() != nil || empty.Layout() != nil {
		t.Fatal("uninitialized configuration produced JSON")
	}
}
