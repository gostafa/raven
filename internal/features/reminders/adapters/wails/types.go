// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package wails adapts reminder delivery to native Wails notifications.
package wails

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"raven/internal/features/reminders/ports"
	resilience "raven/internal/shared/resilience/adapters/retry"
)

type (
	reminderDelivery = struct {
		messages []ports.Messages
		count    int
	}

	// SoundPlayer plays one complete reminder sound.
	SoundPlayer struct{ play func() error }

	// Notifier translates reminder delivery to native notifications.
	Notifier struct {
		operations map[string]func(context.Context, int, func()) notificationResult
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
		operations map[string]func(context.Context, int, func()) notificationResult
	}
	notificationLifecycle = struct {
		native    NativeRuntime
		owner     func() context.Context
		cancel    context.CancelFunc
		warn      func(context.Context, error)
		resource  *resilience.Initialization[bool]
		available *atomic.Bool
		listeners map[uint]func()
		next      uint
		mu        sync.Mutex
		stopped   bool
	}

	notificationResult = struct {
		unsubscribe func()
		failure     error
		allowed     bool
	}
)
