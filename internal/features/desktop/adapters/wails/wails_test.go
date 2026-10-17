package wails

import (
	"reflect"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
)

func TestDesktopWindowConfigurationPreservesMonochromeBehavior(t *testing.T) {
	main := mainWindowOptions(&Configuration{Layout: configurationFixture().Layout})
	if main.Name != "raven" || main.Title != "Raven" || main.URL != "/" || !main.Hidden ||
		main.Width != 960 ||
		main.Height != 680 ||
		main.MinWidth != 620 ||
		main.MinHeight != 520 {
		t.Fatalf("main window changed: %+v", main)
	}
	if main.Mac.Appearance != application.NSAppearanceNameDarkAqua ||
		main.Mac.Backdrop != application.MacBackdropTranslucent {
		t.Fatal("main window appearance changed")
	}
	widget := widgetWindowOptions(&Configuration{Layout: configurationFixture().Layout})
	if widget.Name != "raven-widget" || widget.URL != "/?view=widget" || widget.Hidden ||
		widget.Width != configurationFixture().Layout.WidgetSize ||
		widget.MinWidth != configurationFixture().Layout.WidgetSize ||
		widget.MaxWidth != configurationFixture().Layout.WidgetSize ||
		widget.Height != configurationFixture().Layout.WidgetSize ||
		widget.MinHeight != configurationFixture().Layout.WidgetSize ||
		widget.MaxHeight != configurationFixture().Layout.WidgetSize {
		t.Fatal("widget geometry changed")
	}
	if widget.InitialPosition != application.WindowXY || widget.X != 24 || widget.Y != 100 ||
		widget.BackgroundType != application.BackgroundTypeTransparent ||
		!reflect.DeepEqual(widget.BackgroundColour, application.NewRGBA(255, 255, 255, 0)) {
		t.Fatal("widget background or placement changed")
	}
	if widget.Mac.Backdrop != application.MacBackdropTransparent || !widget.Mac.DisableShadow ||
		widget.Mac.CornerType != application.MacWindowCornerTypeSquare {
		t.Fatal("widget platform appearance changed")
	}
	review := reviewWindowOptions(&Configuration{Layout: configurationFixture().Layout})
	if review.Name != "raven-review" || review.URL != "/?view=review" || !review.Hidden ||
		review.Width != configurationFixture().Layout.ReviewWidth ||
		review.Height != configurationFixture().Layout.ReviewHeight ||
		review.Mac.Appearance != application.NSAppearanceNameDarkAqua {
		t.Fatal("review window changed")
	}
	for _, window := range []application.WebviewWindowOptions{widget, review} {
		if !window.Frameless || !window.DisableResize || !window.AlwaysOnTop ||
			!window.Windows.HiddenOnTaskbar ||
			window.MaximiseButtonState != application.ButtonDisabled ||
			window.FullscreenButtonState != application.ButtonDisabled {
			t.Fatal("floating window interaction changed")
		}
		if window.Mac.WindowClass != application.MacWindowClassPanel ||
			!window.Mac.PanelPreferences.NonActivating ||
			!window.Mac.PanelPreferences.FloatingPanel ||
			window.Mac.WindowLevel != application.MacWindowLevelPopUpMenu {
			t.Fatal("floating panel behavior changed")
		}
	}
	for _, window := range []application.WebviewWindowOptions{main, review} {
		if !reflect.DeepEqual(window.BackgroundColour, application.NewRGB(0, 0, 0)) {
			t.Fatal("native black background changed")
		}
	}
	identity := desktopIdentityOptions([]byte{1})
	if identity.Name != "Raven" ||
		identity.Description != "A little practice. A little more remembered." ||
		len(identity.Icon) == 0 {
		t.Fatal("application identity changed")
	}
}

func configurationFixture() *configuration.Settings {
	settings, err := (configyaml.Loader{}).Load()
	if err != nil {
		panic(err)
	}
	return settings
}

func TestConfiguredIdentityAndGeometryReachNativeOptions(t *testing.T) {
	settings := &Configuration{
		ApplicationID: "custom.desktop",
		Layout:        configurationFixture().Layout,
	}
	settings.Layout.WidgetSize = 64
	settings.Layout.ReviewWidth = 300
	settings.Layout.ReviewHeight = 400
	if singleInstanceOptions(
		&ApplicationDependencies{Configuration: settings},
	).UniqueID != "custom.desktop" {
		t.Fatal("single-instance identity ignored configuration")
	}
	widget, review := widgetWindowOptions(settings), reviewWindowOptions(settings)
	if widget.Width != 64 || widget.Height != 64 || review.Width != 300 || review.Height != 400 {
		t.Fatal("window sizes ignored configuration")
	}
}
