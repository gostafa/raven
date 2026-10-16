//go:build linux && cgo && !android && !server

package native

/*
#cgo !gtk3 pkg-config: gtk4
#cgo gtk3 pkg-config: gtk+-3.0
#include <gtk/gtk.h>

static void raven_style_widget(void *handle) {
    GtkWidget *window = GTK_WIDGET(handle);
    static gboolean installed = FALSE;
    gtk_widget_set_name(window, "raven-widget");
    if (!installed) {
    GtkCssProvider *provider = gtk_css_provider_new();
    const char *css = "#raven-widget { background-color: transparent; "
        "background-image: none; border-radius: 0; box-shadow: none; }";
#if GTK_MAJOR_VERSION >= 4
    gtk_css_provider_load_from_data(provider, css, -1);
    gtk_style_context_add_provider_for_display(gtk_widget_get_display(window),
        GTK_STYLE_PROVIDER(provider), GTK_STYLE_PROVIDER_PRIORITY_APPLICATION + 1);
#else
    gtk_css_provider_load_from_data(provider, css, -1, NULL);
    gtk_style_context_add_provider_for_screen(gtk_widget_get_screen(window),
        GTK_STYLE_PROVIDER(provider), GTK_STYLE_PROVIDER_PRIORITY_APPLICATION + 1);
#endif
    g_object_unref(provider);
    installed = TRUE;
    }
#if GTK_MAJOR_VERSION >= 4
    gtk_widget_set_overflow(window, GTK_OVERFLOW_VISIBLE);
#endif
}
*/
import "C"

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Configure applies transparent styling to the floating widget's native window.
func Configure(window *application.WebviewWindow) error {
	if handle := window.NativeWindow(); handle != nil {
		C.raven_style_widget(handle)
	}
	return nil
}
