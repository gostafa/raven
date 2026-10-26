// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package main provides Raven's public Wails facade and executable composition boundary.
package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/fx"

	transferports "raven/internal/features/cardtransfer/ports"
	companion "raven/internal/features/companion/domain"
	desktopports "raven/internal/features/desktop/ports"
	archiveports "raven/internal/features/folderarchive/ports"
	reminderports "raven/internal/features/reminders/ports"
	libraryports "raven/internal/shared/boltstore/ports"
	mediaports "raven/internal/shared/desktopmedia/ports"
	"raven/internal/shared/lifetime"
	localization "raven/internal/shared/localization/application"
)

type (
	desktopRunner interface {
		Register(service application.Service)
		Run() error
	}
	applicationParams struct {
		fx.In

		Desktop desktopRunner
		Service *RavenService
		Runtime *lifetime.Runtime
	}
	serviceResult[Value any] struct {
		value   Value
		failure error
	}
	serviceParams struct {
		fx.In

		Localization  *localization.Service
		Repository    libraryports.Repository
		Media         mediaports.Selection
		Windows       companion.Windows
		Events        desktopports.Events
		Reminders     reminderports.Control
		Archives      archiveports.Transfer
		Transfers     transferports.Transfer
		Notifications desktopports.Notifications
	}
	// RavenService exposes Raven operations using injected feature ports.
	RavenService struct {
		operations map[string]func(string, string) serviceResult[string]
	}
	// ServiceError exposes a message without retaining an implementation error chain.
	ServiceError struct {
		// Message describes a failed frontend operation.
		Message string `json:"message"`
		// Code is a stable frontend error identifier.
		Code string `json:"code"`
	}
)
