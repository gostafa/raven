package wails

import (
	"reflect"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestDesktopWindowConfigurationPreservesMonochromeBehavior(t *testing.T) {
	main := mainWindowOptions()
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
	widget := widgetWindowOptions()
	if widget.Name != "raven-widget" || widget.URL != "/?view=widget" || widget.Hidden ||
		widget.Width != widgetSize ||
		widget.MinWidth != widgetSize ||
		widget.MaxWidth != widgetSize ||
		widget.Height != widgetSize ||
		widget.MinHeight != widgetSize ||
		widget.MaxHeight != widgetSize {
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
	review := reviewWindowOptions()
	if review.Name != "raven-review" || review.URL != "/?view=review" || !review.Hidden ||
		review.Width != reviewWidth ||
		review.Height != reviewHeight ||
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
