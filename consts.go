// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package main

import (
	"raven/internal/features/companion"
)

const (
	widgetSize   = companion.WidgetSize
	reviewWidth  = companion.ReviewWidth
	reviewHeight = companion.ReviewHeight

	logErrorKey     = "error"
	logOperationKey = "operation"

	resetMenuTitle = "Reset widget position"

	appTitle               = "Raven"
	appName                = "raven"
	startupMenuTitle       = "Start at login"
	openMenuTitle          = "Open Raven"
	quitMenuTitle          = "Quit Raven"
	blackChannel           = 0
	whiteChannel           = 255
	mainWindowWidth        = 960
	mainWindowHeight       = 680
	mainWindowMinWidth     = 620
	mainWindowMinHeight    = 520
	initialWidgetX         = 24
	initialWidgetY         = 100
	firstLaunchArgument    = 1
	startupFailureExitCode = 1
	startupErrorMessage    = "Raven could not update login startup. You can try again from the Raven menu.\n\n"

	cardStorageIsUnavailable   = "card storage is unavailable"
	folderStorageIsUnavailable = "folder storage is unavailable"
	ravenSchedulesChanged      = "raven:schedules-changed"
	zero                       = 0
)
