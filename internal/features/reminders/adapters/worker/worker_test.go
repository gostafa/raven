package worker

import (
	"testing"
	"time"

	cardapi "raven/api/cards"
	reminders "raven/internal/features/reminders/application"
	"raven/internal/features/reminders/ports"
)

type source struct{ reminders.Source }

func (source) ListCards() ([]cardapi.Card, error)     { return nil, nil }
func (source) ClaimNotificationPrompt() (bool, error) { return false, nil }

type notifications struct {
	ports.Notifications
	available bool
}

func (native notifications) Available() bool { return native.available }

func TestReadinessAndShutdownGateReconciliation(t *testing.T) {
	for _, available := range []bool{false, true} {
		published := make(chan struct{}, 10)
		worker := New(source{}, notifications{available: available}, reminders.Callbacks{
			Publish: func() { published <- struct{}{} },
			Warn:    func(message string, err error) { t.Errorf("%s: %v", message, err) },
		})
		worker.Start()
		worker.Wake()
		if worker.coordinator != nil {
			t.Fatal("started before lifecycle enabled reconciliation")
		}
		worker.Enable()
		worker.Start()
		first := worker.coordinator
		worker.Start()
		if first == nil || worker.coordinator != first {
			t.Fatal("readiness must start exactly one coordinator")
		}
		waitPublished(t, published)
		worker.Wake()
		waitPublished(t, published)
		worker.Stop()
		worker.Stop()
		worker.Wake()
		worker.Start()
		if worker.coordinator != first {
			t.Fatal("restarted after shutdown")
		}
	}
	worker := New(nil, nil, reminders.Callbacks{})
	worker.Stop()
	worker.Enable()
	worker.Start()
	if worker.coordinator != nil {
		t.Fatal("late readiness started a stopped worker")
	}
}

func waitPublished(t *testing.T, published <-chan struct{}) {
	t.Helper()
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("reconciliation did not publish")
	}
}
