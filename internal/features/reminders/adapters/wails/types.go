// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package wails adapts reminder delivery to native Wails notifications.
package wails

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type (
	// Notifier translates reminder delivery to native notifications.
	Notifier struct {
		native NativeNotifications
	}

	// NativeNotifications supplies the native operations used by the adapter.
	NativeNotifications interface {
		CheckNotificationAuthorization() (bool, error)
		SendNotification(options notifications.NotificationOptions) error
		RequestNotificationAuthorization() (bool, error)
	}

	// NativeRuntime supplies the notification service lifecycle and responses.
	NativeRuntime interface {
		NativeNotifications
		ServiceStartup(ctx context.Context, options application.ServiceOptions) error
		ServiceShutdown() error
		OnNotificationResponse(callback func(notifications.NotificationResult))
	}

	// Runtime tracks optional native notification availability for managed workers.
	Runtime struct {
		Notifier

		native      NativeRuntime
		logger      *slog.Logger
		logErrorKey string
		available   atomic.Bool
	}
)
