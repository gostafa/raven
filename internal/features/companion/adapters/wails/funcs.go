// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"context"
	"image"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// New constructs the adapter with its required runtime dependencies.
func New(deps *Dependencies) *Windows {
	app, handles, configuration := deps.App, deps.Handles, deps.Configuration
	settings := &deps.Settings
	windows := &Windows{
		settings:        *settings,
		mainWindow:      handles.Main,
		widget:          handles.Widget,
		reviewWindow:    nil,
		reviewPlacement: nil,
		configuration:   configuration,
		invoke:          application.InvokeSync,
		primaryScreen:   app.Screen.GetPrimary,
		quit:            app.Quit,
		logger:          app.Logger,
	}
	windows.bindReview(handles.Review)

	return windows
}

func (windows *Windows) bindReview(review *application.WebviewWindow) {
	windows.reviewWindow, windows.reviewPlacement = nil, nil

	if review != nil {
		windows.reviewWindow, windows.reviewPlacement = review, review
	}
}

// ConfigureWidget configures platform-specific floating widget behavior.
func (windows *Windows) ConfigureWidget() {
	windows.invoke(func() {
		err := windows.configuration.Configure()
		if err != nil {
			windows.logger.ErrorContext(
				context.Background(),
				"could not configure raven widget",
				//nolint:sloglint // Log field names come from validated embedded configuration.
				slog.Any(windows.settings.LogErrorKey, err),
			)
		}
	})
}

// ShowMain shows, restores, and focuses the main library window.
func (windows *Windows) ShowMain() {
	windows.mainWindow.Show()
	windows.mainWindow.Restore()
	windows.mainWindow.Focus()
}

// HideMain hides the library and keeps the widget visible.
func (windows *Windows) HideMain() {
	windows.widget.Show()
	windows.mainWindow.Hide()
}

// ResetWidget repositions the widget within the primary screen.
func (windows *Windows) ResetWidget() {
	if screen := windows.primaryScreen(); screen != nil {
		area := screen.WorkArea
		windows.widget.SetPosition(
			area.X+max(zero, area.Width-windows.settings.Layout.WidgetSize-widgetMargin),
			area.Y+max(zero, area.Height-windows.settings.Layout.WidgetSize-widgetMargin),
		)
	}

	windows.widget.Show()
	windows.PositionFloatingReview()
}

// Quit is intentionally separate from closing the main window.
func (windows *Windows) Quit() {
	windows.quit()
}

// PositionFloatingReview repositions the visible review window beside the widget.
func (windows *Windows) PositionFloatingReview() {
	if windows.reviewWindow == nil || !windows.reviewWindow.IsVisible() {
		return
	}

	windows.placeFloatingReview()
}

func (windows *Windows) placeFloatingReview() {
	screen := windows.widgetScreen()

	if screen == nil || screen.WorkArea.Width <= zero || screen.WorkArea.Height <= zero {
		return
	}

	horizontal, vertical := windows.widget.Position()
	area := screen.WorkArea
	bounds := windows.settings.Layout.Bounds(
		horizontal,
		vertical,
		new(image.Rect(area.X, area.Y, area.X+area.Width, area.Y+area.Height)),
	)
	windows.reviewPlacement.SetSize(bounds.Dx(), bounds.Dy())
	windows.reviewPlacement.SetPosition(bounds.Min.X, bounds.Min.Y)
}

func (windows *Windows) widgetScreen() *application.Screen {
	screen, err := windows.widget.GetScreen()
	if err != nil || screen == nil {
		return windows.primaryScreen()
	}

	return screen
}

// RequestFloatingReviewClose asks the frontend to close after pending saves finish.
func (windows *Windows) RequestFloatingReviewClose() {
	// The frontend owns the saving state and ignores close requests until a
	// rating finishes. DispatchWailsEvent targets only this window.
	windows.reviewWindow.DispatchWailsEvent(
		&application.CustomEvent{Name: windows.settings.CloseFloatingReview, Data: nil, Sender: ""},
	)
}

// ToggleFloatingReview opens the companion card without showing the library.
func (windows *Windows) ToggleFloatingReview() {
	windows.invoke(func() {
		if windows.reviewWindow.IsVisible() {
			windows.RequestFloatingReviewClose()

			return
		}

		windows.placeFloatingReview()
		windows.reviewWindow.Show()
		windows.reviewWindow.DispatchWailsEvent(
			&application.CustomEvent{
				Name:   windows.settings.OpenFloatingReview,
				Data:   nil,
				Sender: "",
			},
		)
		windows.reviewWindow.Focus()
	})
}

// HideFloatingReview is called after the review UI has stopped media playback.
func (windows *Windows) HideFloatingReview() {
	windows.reviewWindow.Hide()
}
