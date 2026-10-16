package reminders

import (
	"errors"
	"testing"
	"time"

	"raven/internal/library"
)

type testSource struct {
	cards                       []library.Card
	listErr, promptErr, markErr error
	prompt                      bool
	marked                      int
}

func (source *testSource) ListCards() ([]library.Card, error) { return source.cards, source.listErr }

func (source *testSource) ClaimNotificationPrompt() (bool, error) {
	return source.prompt, source.promptErr
}

func (source *testSource) MarkNotified([]library.DueOccurrence) error {
	source.marked++
	return source.markErr
}

type testNotifier struct {
	allowed                         bool
	checkErr, sendErr, authorizeErr error
	sent, requested                 int
}

func (notifier *testNotifier) CheckNotificationAuthorization() (bool, error) {
	return notifier.allowed, notifier.checkErr
}

func (notifier *testNotifier) Send(count int) error {
	notifier.sent += count
	return notifier.sendErr
}

func (notifier *testNotifier) RequestAuthorization() error {
	notifier.requested++
	return notifier.authorizeErr
}

func TestDeliveryFailuresAreReportedWithoutFalseAcknowledgments(t *testing.T) {
	failure := errors.New("native operation failed")
	for _, phase := range []string{"check", "send", "acknowledge"} {
		t.Run(phase, func(t *testing.T) {
			source := new(testSource)
			notifier := &testNotifier{allowed: true}
			switch phase {
			case "check":
				notifier.checkErr = failure
			case "send":
				notifier.sendErr = failure
			case "acknowledge":
				source.markErr = failure
			}
			warnings := 0
			coordinator := Coordinator{
				source: source, notifier: notifier,
				callbacks: Callbacks{Warn: func(message string, err error) {
					warnings++
					if message == "" || !errors.Is(err, failure) {
						t.Errorf("unexpected warning: %q, %v", message, err)
					}
				}},
			}
			coordinator.deliver(
				[]library.DueOccurrence{
					{FolderID: "folder", CardID: "card", ItemID: "item", Due: 1},
				},
			)
			if warnings != 1 {
				t.Fatalf("expected one warning, got %d", warnings)
			}
			wantMarked := 0
			if phase == "acknowledge" {
				wantMarked = 1
			}
			if source.marked != wantMarked {
				t.Fatalf("unexpected acknowledgments: %d", source.marked)
			}
		})
	}
}

func TestAuthorizationFailureWakesReconciliationAndDoesNotRepeatPrompt(t *testing.T) {
	failure := errors.New("authorization unavailable")
	notifier := &testNotifier{authorizeErr: failure}
	warnings := 0
	coordinator := Coordinator{
		changed: make(chan struct{}, 1), notifier: notifier,
		callbacks: Callbacks{Warn: func(_ string, err error) {
			warnings++
			if !errors.Is(err, failure) {
				t.Error(err)
			}
		}},
	}
	coordinator.authorize()
	if warnings != 1 || notifier.requested != 1 || len(coordinator.changed) != 1 {
		t.Fatal("authorization failure was not reported and reconciled")
	}
	for _, source := range []*testSource{{promptErr: failure}, {prompt: false}} {
		coordinator.source = source
		coordinator.requestAuthorization()
	}
	if notifier.requested != 1 {
		t.Fatal("authorization prompt repeated")
	}
	coordinator.notifier = nil
	coordinator.requestAuthorization()
}

func TestReconciliationReadFailureAndTimerSignals(t *testing.T) {
	failure := errors.New("storage unavailable")
	source := &testSource{listErr: failure}
	warnings, publications := 0, 0
	coordinator := Coordinator{
		source: source, stopping: make(chan struct{}), changed: make(chan struct{}, 1),
		callbacks: Callbacks{
			Publish: func() { publications++ },
			Warn: func(_ string, err error) {
				warnings++
				if !errors.Is(err, failure) {
					t.Error(err)
				}
			},
		},
	}
	if delay := coordinator.reconcile(); delay != pollInterval || warnings != 1 ||
		publications != 0 {
		t.Fatal("read failure published a partial schedule")
	}
	source.listErr = nil
	if delay := coordinator.reconcile(); delay != pollInterval || publications != 1 {
		t.Fatal("empty schedule did not reconcile")
	}
	if !coordinator.wait(time.Nanosecond) {
		t.Fatal("timer did not request reconciliation")
	}
	coordinator.Wake()
	if !coordinator.wait(time.Hour) {
		t.Fatal("wake did not request reconciliation")
	}
	close(coordinator.stopping)
	if coordinator.wait(time.Hour) {
		t.Fatal("stop did not end reconciliation")
	}
}

func TestCoordinatorRunsWithoutNativeNotifications(t *testing.T) {
	published := make(chan struct{}, 1)
	coordinator := Start(new(testSource), nil, Callbacks{
		Publish: func() {
			select {
			case published <- struct{}{}:
			default:
			}
		},
		Warn: func(message string, err error) { t.Errorf("%s: %v", message, err) },
	})
	defer coordinator.Stop()
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("schedule reconciliation did not run")
	}
	coordinator.Stop()
	coordinator.Stop()
}
