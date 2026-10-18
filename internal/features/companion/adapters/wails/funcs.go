// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"context"
	"image"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	companion "raven/internal/features/companion/domain"
)

// New constructs window capabilities without starting native resources.
func New(deps *Dependencies, settings *Settings) *Windows {
	state := newWindowState(deps, settings)

	return newWindowOperations(state)
}

func newWindowState(deps *Dependencies, settings *Settings) *windowState {
	var state windowState

	bindMainWindow(&state, deps.Main)
	bindWidgetWindow(&state, deps.Widget)
	bindWindowRuntime(&state, deps)
	bindWindowSettings(&state, settings)
	bindReview(&state, deps.Review)

	return &state
}

func bindMainWindow(state *windowState, window mainWindow) {
	state.showMain, state.hideMain = window.Show, window.Hide
	state.restoreMain, state.focusMain = window.Restore, window.Focus
}

func bindWidgetWindow(state *windowState, window widgetSurface) {
	state.showWidget, state.widgetPosition = window.Show, window.Position
	state.setWidgetPosition, state.getWidgetScreen = window.SetPosition, window.GetScreen
}

func bindWindowRuntime(state *windowState, deps *Dependencies) {
	state.configure, state.invoke = deps.Configure, application.InvokeSync
	state.primaryScreen, state.quit = deps.App.Screen.GetPrimary, deps.App.Quit
	state.logger = deps.App.Logger
}

func bindWindowSettings(state *windowState, settings *Settings) {
	state.logErrorKey, state.openFloatingReview = settings.LogErrorKey, settings.OpenFloatingReview
	state.closeFloatingReview = settings.CloseFloatingReview
	state.widgetSize, state.reviewWidth = settings.WidgetSize, settings.ReviewWidth
	state.reviewHeight, state.reviewGap = settings.ReviewHeight, settings.ReviewGap
}

func newWindowOperations(state *windowState) *Windows {
	return &Windows{operations: map[string]func(){
		"configure": func() { ConfigureWidget(state) }, "show": func() { ShowMain(state) },
		"hide": func() { HideMain(state) }, "reset": func() { ResetWidget(state) },
		"position":   func() { PositionFloatingReview(state) },
		"close":      func() { RequestFloatingReviewClose(state) },
		"toggle":     func() { ToggleFloatingReview(state) },
		"hideReview": func() { HideFloatingReview(state) }, "quit": func() { Quit(state) },
	}}
}

func bindReview(state *windowState, review *application.WebviewWindow) {
	if review == nil {
		return
	}

	bindReviewSurface(state, review)
	bindReviewFrame(state, review)
}

func bindReviewSurface(state *windowState, window reviewSurface) {
	state.showReview, state.hideReview = window.Show, window.Hide
	state.focusReview, state.reviewVisible = window.Focus, window.IsVisible
	state.dispatchReview = window.DispatchWailsEvent
}

func bindReviewFrame(state *windowState, window reviewFrame) {
	state.setReviewSize, state.setReviewPosition = window.SetSize, window.SetPosition
}

// ConfigureWidget applies platform-specific widget behavior.
func (windows *Windows) ConfigureWidget() { windows.operations["configure"]() }

// ShowMain shows, restores, and focuses the library.
func (windows *Windows) ShowMain() { windows.operations["show"]() }

// HideMain hides the library while retaining the launcher.
func (windows *Windows) HideMain() { windows.operations["hide"]() }

// ResetWidget places the launcher on the primary screen.
func (windows *Windows) ResetWidget() { windows.operations["reset"]() }

// PositionFloatingReview places a visible review beside the launcher.
func (windows *Windows) PositionFloatingReview() { windows.operations["position"]() }

// RequestFloatingReviewClose lets the frontend finish pending saves.
func (windows *Windows) RequestFloatingReviewClose() { windows.operations["close"]() }

// ToggleFloatingReview opens a review or requests its closure.
func (windows *Windows) ToggleFloatingReview() { windows.operations["toggle"]() }

// HideFloatingReview hides a review after media playback stops.
func (windows *Windows) HideFloatingReview() { windows.operations["hideReview"]() }

// Quit exits the native application.
func (windows *Windows) Quit() { windows.operations["quit"]() }

// ConfigureWidget configures platform-specific floating widget behavior.
func ConfigureWidget(windows *windowState) {
	windows.invoke(func() {
		err := windows.configure()
		if err != nil {
			windows.logger.ErrorContext(
				context.Background(),
				"could not configure raven widget",
				//nolint:sloglint // Log field names come from validated embedded configuration.
				slog.Any(windows.logErrorKey, err),
			)
		}
	})
}

// ShowMain shows, restores, and focuses the main library window.
func ShowMain(windows *windowState) {
	windows.showMain()
	windows.restoreMain()
	windows.focusMain()
}

// HideMain hides the library and keeps the widget visible.
func HideMain(windows *windowState) {
	windows.showWidget()
	windows.hideMain()
}

// ResetWidget repositions the widget within the primary screen.
func ResetWidget(windows *windowState) {
	if screen := windows.primaryScreen(); screen != nil {
		area := screen.WorkArea
		windows.setWidgetPosition(
			area.X+max(zero, area.Width-windows.widgetSize-widgetMargin),
			area.Y+max(zero, area.Height-windows.widgetSize-widgetMargin),
		)
	}

	windows.showWidget()
	PositionFloatingReview(windows)
}

// Quit is intentionally separate from closing the main window.
func Quit(windows *windowState) {
	windows.quit()
}

// PositionFloatingReview repositions the visible review window beside the widget.
func PositionFloatingReview(windows *windowState) {
	if windows.reviewVisible == nil || !windows.reviewVisible() {
		return
	}

	placeFloatingReview(windows)
}

func placeFloatingReview(windows *windowState) {
	screen := widgetScreen(windows)

	if screen == nil || screen.WorkArea.Width <= zero || screen.WorkArea.Height <= zero {
		return
	}

	horizontal, vertical := windows.widgetPosition()
	area := screen.WorkArea
	bounds := windowLayout(windows).Bounds(
		horizontal,
		vertical,
		new(image.Rect(area.X, area.Y, area.X+area.Width, area.Y+area.Height)),
	)
	windows.setReviewSize(bounds.Dx(), bounds.Dy())
	windows.setReviewPosition(bounds.Min.X, bounds.Min.Y)
}

func widgetScreen(windows *windowState) *application.Screen {
	screen, err := windows.getWidgetScreen()
	if err != nil || screen == nil {
		return windows.primaryScreen()
	}

	return screen
}

// RequestFloatingReviewClose asks the frontend to close after pending saves finish.
func RequestFloatingReviewClose(windows *windowState) {
	// The frontend owns the saving state and ignores close requests until a
	// rating finishes. DispatchWailsEvent targets only this window.
	windows.dispatchReview(
		&application.CustomEvent{Name: windows.closeFloatingReview, Data: nil, Sender: ""},
	)
}

// ToggleFloatingReview opens the companion card without showing the library.
func ToggleFloatingReview(windows *windowState) {
	windows.invoke(func() {
		if windows.reviewVisible() {
			RequestFloatingReviewClose(windows)

			return
		}

		placeFloatingReview(windows)
		windows.showReview()
		windows.dispatchReview(
			&application.CustomEvent{
				Name:   windows.openFloatingReview,
				Data:   nil,
				Sender: "",
			},
		)
		windows.focusReview()
	})
}

// HideFloatingReview is called after the review UI has stopped media playback.
func HideFloatingReview(windows *windowState) {
	windows.hideReview()
}

func windowLayout(state *windowState) *companion.Layout {
	return &companion.Layout{
		WidgetSize: state.widgetSize, ReviewWidth: state.reviewWidth,
		ReviewHeight: state.reviewHeight, ReviewGap: state.reviewGap,
	}
}
