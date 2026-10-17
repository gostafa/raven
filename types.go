// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package main provides Raven's public Wails facade and executable composition boundary.
package main

import (
	"go.uber.org/fx"

	companion "raven/internal/features/companion/domain"
	configuration "raven/internal/features/configuration/domain"
	desktopports "raven/internal/features/desktop/ports"
	reminderports "raven/internal/features/reminders/ports"
	libraryports "raven/internal/shared/boltstore/ports"
	mediaports "raven/internal/shared/desktopmedia/ports"
)

type (
	serviceResult[Value any] struct {
		value   Value
		failure error
	}
	serviceParams struct {
		fx.In

		Repository libraryports.Repository
		Media      mediaports.Selection
		Windows    companion.Windows
		Events     desktopports.Events
		Reminders  reminderports.Control
	}
	// RavenService exposes Raven operations using injected feature ports.
	RavenService struct {
		configuration *configuration.Settings
		store         libraryports.Repository
		media         mediaports.Selection
		windows       companion.Windows
		events        desktopports.Events
		reminders     reminderports.Control
	}
	// ServiceError exposes a message without retaining an implementation error chain.
	ServiceError struct{ Message string }
)
