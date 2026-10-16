// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package companion defines floating-window geometry and window ports.
package companion

type Rect struct{ X, Y, Width, Height int }

type Windows interface {
	ConfigureWidget()
	ShowMain()
	HideMain()
	ResetWidget()
	Quit()
	PositionFloatingReview()
	RequestFloatingReviewClose()
	ToggleFloatingReview()
	HideFloatingReview()
}
