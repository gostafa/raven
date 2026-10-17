package composition

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	configuration "raven/internal/features/configuration/domain"
	"raven/internal/shared/lifetime"
)

type eventRecorder struct{ name string }

func (recorder *eventRecorder) Emit(name string) { recorder.name = name }

func TestNativeProviderAndConfiguredCallbacks(t *testing.T) {
	if newNative() == nil {
		t.Fatal("missing native notifications")
	}
	var output bytes.Buffer
	events := &eventRecorder{}
	runtime := lifetime.New()
	defer runtime.Cancel()
	settings := &configuration.Settings{
		SchedulesChanged: "custom:schedules", LogOperationKey: "operation", LogErrorKey: "failure",
	}
	callbacks := newCallbacks(callbacksParams{
		Events: events, Runtime: runtime,
		Logger: slog.New(slog.NewTextHandler(&output, nil)),
	}, settings)
	callbacks.Publish()
	if events.name != settings.SchedulesChanged {
		t.Fatal("wrong event")
	}
	callbacks.Warn("read schedules", errors.New("storage unavailable"))
	if !strings.Contains(output.String(), "operation=\"read schedules\"") ||
		!strings.Contains(output.String(), "failure=\"storage unavailable\"") {
		t.Fatalf("missing configured log details: %s", &output)
	}
}
