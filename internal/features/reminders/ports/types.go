// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines ports contracts for reminders.
package ports

import (
	"context"
	"log/slog"
)

type (
	// Availability reports whether native initialization succeeded.
	Availability interface{ Available() bool }
	// AvailabilityChanges publishes native initialization completion.
	AvailabilityChanges interface {
		SubscribeAvailability(changed func()) func()
	}
	// RuntimeOptions supplies notification diagnostics and localized messages.
	RuntimeOptions = struct {
		Logger   *slog.Logger
		Warn     func(context.Context, error)
		Messages Messages
	}
	// Sound plays one complete reminder sound before returning.
	Sound interface {
		Play() error
	}
	// Control starts reconciliation on desktop readiness and wakes or stops its worker.
	Control interface {
		Start()
		Wake()
		Stop()
	}
	// Store acknowledges a JSON batch of due occurrences after notification delivery.
	Store interface {
		MarkNotified(occurrences string) error
	}
	// Notifier provides notification authorization and reminder delivery.
	Notifier interface {
		CheckNotificationAuthorization() (bool, error)
		Send(count int) error
		RequestAuthorization() (bool, error)
	}

	// Messages supplies localized native text without exposing a translation library.
	Messages interface {
		Text(key string, count int) string
	}
)
