// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package wails

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

func reminderSound() *notifications.NotificationSound {
	return &notifications.NotificationSound{Silent: true, Name: ""}
}

func recordNotificationStartupCleanupFailure(
	_ *notificationLifecycle,
	_ error,
) {
}

func playReminder(ctx context.Context, resolve func() (string, error)) error {
	executable, err := resolve()
	if err != nil {
		return fmt.Errorf("locate reminder sound: %w", err)
	}

	return errors.Join(
		playFile(
			ctx,
			filepath.Join(filepath.Dir(executable), "..", "Resources", "raven.wav"),
			(*exec.Cmd).Run,
		),
	)
}

func playFile(ctx context.Context, path string, run func(*exec.Cmd) error) error {
	bounded, cancel := context.WithTimeout(ctx, playbackTimeout)
	defer cancel()

	//nolint:gosec // The executable is fixed; the bundled audio path is passed as one argument.
	err := run(exec.CommandContext(bounded, "/usr/bin/afplay", path))
	if err != nil {
		return fmt.Errorf("play Raven reminder: %w", err)
	}

	return nil
}
