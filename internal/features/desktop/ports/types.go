// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package ports defines ports contracts for desktop.
package ports

type (
	// Notifications delivers application messages through the operating system.
	Notifications interface {
		Notify(title, body string) error
	}

	// Events publishes frontend notifications.
	Events interface{ Emit(name string) }

	// OpenRequests supplies queued launch visibility effects.
	OpenRequests interface{ Pending() <-chan struct{} }

	// Startup controls login registration after desktop readiness.
	Startup interface {
		Initialize() error
		Toggle() error
		Refresh() error
		SetPublisher(publish func(bool, bool))
	}
	// LaunchVisibility reconciles readiness and explicit requests to open the library.
	LaunchVisibility interface {
		ApplicationStarted()
		RequestOpen()
		SecondInstance(args []string)
	}

	// Messages supplies localized native text without exposing a translation library.
	Messages interface {
		Text(key string, count int) string
	}

	// Localization observes language changes and follows changed OS preferences.
	Localization interface {
		Messages
		Subscribe(listener func()) func()
		Refresh()
	}
)
