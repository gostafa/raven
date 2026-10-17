package application

import (
	"errors"
	"testing"
	"time"

	cardapi "raven/api/cards"
	reviewapi "raven/api/reviews"
)

type testSource struct {
	cards                       []cardapi.Card
	listErr, promptErr, markErr error
	prompt                      bool
	marked                      int
}

func (source *testSource) ListCards() ([]cardapi.Card, error) { return source.cards, source.listErr }

func (source *testSource) ClaimNotificationPrompt() (bool, error) {
	return source.prompt, source.promptErr
}

func (source *testSource) MarkNotified([]reviewapi.DueOccurrence) error {
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

func (notifier *testNotifier) RequestAuthorization() (bool, error) {
	notifier.requested++
	return notifier.allowed, notifier.authorizeErr
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
				[]reviewapi.DueOccurrence{
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

func TestAuthorizationOutcomeOnlyWakesForGrantedPermission(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		notifier := &testNotifier{allowed: allowed}
		coordinator := Coordinator{
			changed:  make(chan struct{}, 1),
			notifier: notifier,
			callbacks: Callbacks{
				Warn: func(message string, err error) { t.Errorf("%s: %v", message, err) },
			},
		}
		coordinator.authorize()
		if (len(coordinator.changed) != 0) != allowed || notifier.requested != 1 {
			t.Fatalf(
				"permission=%v wake=%d requests=%d",
				allowed,
				len(coordinator.changed),
				notifier.requested,
			)
		}
	}
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

type source struct{ Source }

func (source) ListCards() ([]cardapi.Card, error)     { return nil, nil }
func (source) ClaimNotificationPrompt() (bool, error) { return false, nil }

type notifications struct {
	Notifications
	available bool
}

func (native notifications) Available() bool { return native.available }

func TestReadinessAndShutdownGateReconciliation(t *testing.T) {
	for _, available := range []bool{false, true} {
		published := make(chan struct{}, 10)
		worker := NewWorker(source{}, notifications{available: available}, Callbacks{
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
	worker := NewWorker(nil, nil, Callbacks{})
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
