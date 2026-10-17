// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package composition defines composition contracts for reminders.
package composition

import (
	"log/slog"

	"go.uber.org/fx"

	desktopports "raven/internal/features/desktop/ports"
	reminderwails "raven/internal/features/reminders/adapters/wails"
	reminders "raven/internal/features/reminders/application"
	reminderports "raven/internal/features/reminders/ports"
	"raven/internal/shared/lifetime"
)

type (
	notificationParams struct {
		fx.In

		Lifecycle fx.Lifecycle
		Source    reminders.Source
		Native    reminderwails.NativeRuntime
		Logger    *slog.Logger
		Runtime   *lifetime.Runtime
	}
	workerParams struct {
		fx.In

		Lifecycle     fx.Lifecycle
		Source        reminders.Source
		Notifications reminderports.Notifications
		Callbacks     reminders.Callbacks
	}
	callbacksParams struct {
		fx.In

		Events  desktopports.Events
		Logger  *slog.Logger
		Runtime *lifetime.Runtime
	}
)
