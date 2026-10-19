package wails

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

func TestNativeBannerDoesNotPlaySystemSound(t *testing.T) {
	sound := reminderOptions(1).Sound
	if sound == nil || !sound.Silent || sound.Name != "" {
		t.Fatal("native banner can fall back to system sound")
	}
}

func TestPlaybackCancellationAndMissingAssetAreReported(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := NewSound(ctx).Play(); err == nil {
		t.Fatal("canceled playback succeeded")
	}
	if err := playFile(t.Context(), "missing-raven.wav", (*exec.Cmd).Run); err == nil {
		t.Fatal("missing audio succeeded")
	}
}

func TestPlaybackReportsExecutableFailureAndSuccessfulCommand(t *testing.T) {
	failure := errors.New("executable unavailable")
	if err := playReminder(
		t.Context(),
		func() (string, error) { return "", failure },
	); !errors.Is(
		err,
		failure,
	) {
		t.Fatal(err)
	}
	if err := playFile(t.Context(), "raven.wav", func(command *exec.Cmd) error {
		if command.Path != "/usr/bin/afplay" || len(command.Args) != 2 ||
			command.Args[1] != "raven.wav" {
			t.Fatalf("incorrect audio command: %v", command.Args)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
