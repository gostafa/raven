// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package main provides Raven desktop startup and its public Wails facade.
package main

import (
	"net/http"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"raven/internal/autostart"
	"raven/internal/companion"
	"raven/internal/library/ports"
	"raven/internal/reminders"
)

type (
	// RavenService owns the windows and folder database for the lifetime of the app.
	RavenService struct {
		app          *application.App
		mainWindow   *application.WebviewWindow
		widget       *application.WebviewWindow
		reviewWindow *application.WebviewWindow
		store        ports.Repository
		media        ports.Media
		windows      companion.Windows
		mediaHandler http.Handler
		notifier     *notifications.NotificationService
		reminders    *reminders.Coordinator
		reminderMu   sync.Mutex
	}

	lifecycleAdapter struct {
		service   *RavenService
		openStore func() (storageSession, error)
	}
	storageSession struct {
		repository ports.Repository
		media      http.Handler
	}

	// ServiceError exposes a message without retaining an implementation error chain.
	ServiceError       struct{ Message string }
	desktopComposition struct {
		service    *RavenService
		visibility *autostart.Visibility
		startup    *autostart.Controller
	}
)
