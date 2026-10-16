// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package main

import (
	"embed"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

var (
	_ notificationRuntime         = (*notifications.NotificationService)(nil)
	_ notificationResponses       = (*notifications.NotificationService)(nil)
	_ application.ServiceStartup  = (*lifecycleAdapter)(nil)
	_ application.ServiceShutdown = (*lifecycleAdapter)(nil)
	//go:embed all:frontend/dist
	assets embed.FS

	//go:embed build/appicon.png
	appIcon []byte
)
