// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package main provides Raven desktop startup and its public Wails facade.
package main

import (
	"context"
	"net/http"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	autostart "raven/internal/features/autostart/application"
	companion "raven/internal/features/companion/domain"
	libraryports "raven/internal/features/library/ports"
	reminderwails "raven/internal/features/reminders/adapters/wails"
	reminders "raven/internal/features/reminders/application"
)

type (
	notificationRuntime interface {
		reminderwails.NativeNotifications
		ServiceStartup(ctx context.Context, options application.ServiceOptions) error
		ServiceShutdown() error
	}

	notificationResponses interface {
		OnNotificationResponse(callback func(notifications.NotificationResult))
	}

	serviceResult[Value any] struct {
		value   Value
		failure error
	}

	// RavenService owns the windows and folder database for the lifetime of the app.
	RavenService struct {
		app          *application.App
		mainWindow   *application.WebviewWindow
		widget       *application.WebviewWindow
		reviewWindow *application.WebviewWindow
		store        libraryports.Repository
		media        libraryports.Media
		windows      companion.Windows
		mediaHandler http.Handler
		notifier     notificationRuntime
		reminders    *reminders.Coordinator
		reminderMu   sync.Mutex
	}

	lifecycleAdapter struct {
		service   *RavenService
		openStore func() (storageSession, error)
	}
	storageSession struct {
		repository libraryports.Repository
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
