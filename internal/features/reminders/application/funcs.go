// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"encoding/json"
	"fmt"
	"time"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	reviewapi "raven/api/reviews"
	"raven/internal/features/reminders/ports"
)

// Snapshot returns unacknowledged due items and the next reminder delay.
func Snapshot(cards []cardapi.Card, now time.Time) ([]reviewapi.DueOccurrence, time.Duration) {
	pending := []reviewapi.DueOccurrence{}
	delay := pollInterval
	for index := range cards {
		occurrences, next := cardSnapshot(&cards[index], now)
		pending = append(pending, occurrences...)
		delay = min(delay, next)
	}
	return pending, delay
}

func cardSnapshot(card *cardapi.Card, now time.Time) ([]reviewapi.DueOccurrence, time.Duration) {
	pending := []reviewapi.DueOccurrence{}
	delay := pollInterval
	for _, item := range contentapi.Items(card.Items) {
		occurrence, next := itemSnapshot(card, &item, now)
		delay = min(delay, next)
		if occurrence != nil {
			pending = append(pending, *occurrence)
		}
	}
	return pending, delay
}

func itemSnapshot(
	card *cardapi.Card,
	item *contentapi.Item,
	now time.Time,
) (*reviewapi.DueOccurrence, time.Duration) {
	due := contentapi.Schedule(item.Schedule).Due
	if due.After(now) {
		return nil, due.Sub(now)
	}
	dueMillis := due.UnixMilli()
	if item.LastNotifiedDue == dueMillis {
		return nil, pollInterval
	}
	return &reviewapi.DueOccurrence{
		FolderID: card.FolderID,
		CardID:   card.ID,
		ItemID:   item.ID,
		Due:      dueMillis,
	}, pollInterval
}

// DecodeCards validates serialized persisted cards before schedule reconciliation.
func DecodeCards(data string) ([]cardapi.Card, error) {
	var cards []cardapi.Card
	err := json.Unmarshal([]byte(data), &cards)
	if err != nil {
		return nil, fmt.Errorf("decode reminder cards: %w", err)
	}
	for index := range cards {
		err = cardapi.Normalize(&cards[index])
		if err != nil {
			return nil, fmt.Errorf("validate reminder card: %w", err)
		}
	}
	return cards, nil
}

// Deliver sends an authorized reminder before acknowledging unchanged schedules.
func Deliver(store ports.Store, notifier ports.Notifier, pending []reviewapi.DueOccurrence) error {
	allowed, err := notifier.CheckNotificationAuthorization()
	if err != nil {
		return fmt.Errorf("check reminder authorization: %w", err)
	}

	if !allowed {
		return nil
	}

	err = deliverAuthorized(store, notifier, pending)
	if err != nil {
		return fmt.Errorf("deliver authorized reminder: %w", err)
	}

	return nil
}

func deliverAuthorized(
	store ports.Store,
	notifier ports.Notifier,
	pending []reviewapi.DueOccurrence,
) error {
	err := notifier.Send(len(pending))
	if err != nil {
		return fmt.Errorf("send review reminder: %w", err)
	}

	err = acknowledgeDelivery(store, pending)
	if err != nil {
		return fmt.Errorf("acknowledge review reminder: %w", err)
	}

	return nil
}

// Wake requests an immediate reminder reconciliation.
func (coordinator *Coordinator) Wake() { coordinator.wake() }

// Stop cancels reconciliation and waits for its current iteration to finish.
func (coordinator *Coordinator) Stop() { coordinator.stop() }

// Start begins reconciliation with injected persistence and notification effects.
func Start(source Source, notifier ports.Notifier, callbacks Callbacks) *Coordinator {
	signals := &coordinatorSignals{
		stopping: make(chan struct{}),
		changed:  make(chan struct{}, wakeBuffer),
		done:     make(chan struct{}),
	}
	runtime := newReconciliation(source, notifier, callbacks, signals)
	go runReconciliation(runtime)
	return &Coordinator{wake: runtime.wake, stop: func() { stopCoordinator(signals) }}
}

func newReconciliation(
	source Source,
	notifier ports.Notifier,
	callbacks Callbacks,
	signals *coordinatorSignals,
) *reconciliation {
	runtime := &reconciliation{
		list: source.ListCards, claim: source.ClaimNotificationPrompt,
		publish: callbacks.Publish, warn: callbacks.Warn,
		wake:   func() { wakeCoordinator(signals.changed) },
		wait:   func(delay time.Duration) bool { return waitCoordinator(signals, delay) },
		finish: func() { close(signals.done) },
	}
	bindNotifications(runtime, source, notifier)
	return runtime
}

func bindNotifications(runtime *reconciliation, source Source, notifier ports.Notifier) {
	if notifier == nil {
		return
	}
	runtime.authorize = notifier.RequestAuthorization
	runtime.deliver = func(encoded []byte) error {
		return deliverEncoded(source, notifier, encoded)
	}
}

func deliverEncoded(source ports.Store, notifier ports.Notifier, encoded []byte) error {
	var pending []reviewapi.DueOccurrence
	err := json.Unmarshal(encoded, &pending)
	if err != nil {
		return fmt.Errorf("decode due occurrences: %w", err)
	}
	return Deliver(source, notifier, pending)
}

func wakeCoordinator(changed chan struct{}) {
	select {
	case changed <- struct{}{}:
	default:
	}
}

func stopCoordinator(signals *coordinatorSignals) {
	signals.stopOnce.Do(func() { close(signals.stopping) })
	<-signals.done
}

func runReconciliation(runtime *reconciliation) {
	defer runtime.finish()
	requestAuthorization(runtime)
	for {
		delay := reconcile(runtime)
		if !runtime.wait(delay) {
			return
		}
	}
}

func requestAuthorization(runtime *reconciliation) {
	if runtime.authorize == nil {
		return
	}
	prompt, err := runtime.claim()
	if err != nil || !prompt {
		return
	}
	go authorize(runtime)
}

func authorize(runtime *reconciliation) {
	allowed, err := runtime.authorize()
	if err != nil {
		runtime.warn("Notification authorization unavailable", err)
	}
	if allowed || err != nil {
		runtime.wake()
	}
}

func reconcile(runtime *reconciliation) time.Duration {
	cards, err := reconciliationCards(runtime.list)
	if err != nil {
		runtime.warn("Could not read review schedules", err)
		return pollInterval
	}
	pending, delay := Snapshot(cards, time.Now())
	runtime.publish()
	deliverPending(runtime, pending)
	return delay
}

func reconciliationCards(list func() (string, error)) ([]cardapi.Card, error) {
	encoded, err := list()
	if err != nil {
		return nil, fmt.Errorf("read reminder cards: %w", err)
	}
	return DecodeCards(encoded)
}

func deliverPending(runtime *reconciliation, pending []reviewapi.DueOccurrence) {
	if len(pending) == zero || runtime.deliver == nil {
		return
	}
	err := encodeDelivery(runtime.deliver, pending)
	if err != nil {
		runtime.warn("Could not deliver review reminder", err)
	}
}

func encodeDelivery(deliver func([]byte) error, pending []reviewapi.DueOccurrence) error {
	encoded, err := json.Marshal(pending)
	if err != nil {
		return fmt.Errorf("encode due occurrences: %w", err)
	}
	return deliver(encoded)
}

func waitCoordinator(signals *coordinatorSignals, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-signals.stopping:
		return false
	case <-signals.changed:
		return true
	case <-timer.C:
		return true
	}
}

// NewWorker constructs the readiness gate without starting background work.
func NewWorker(source Source, notifications Notifications, callbacks Callbacks) *Worker {
	state := &workerState{start: func() (func(), func()) {
		coordinator := startAvailable(source, notifications, callbacks)
		return coordinator.Wake, coordinator.Stop
	}}
	return &Worker{execute: func(operation string) { executeWorker(state, operation) }}
}

func startAvailable(source Source, notifications Notifications, callbacks Callbacks) *Coordinator {
	var notifier ports.Notifier
	if notifications.Available() {
		notifier = notifications
	}
	return Start(source, notifier, callbacks)
}

// Enable permits readiness events to start reconciliation.
func (worker *Worker) Enable() { worker.execute("enable") }

// Start starts reconciliation once after lifecycle enablement.
func (worker *Worker) Start() { worker.execute("start") }

// Wake requests reconciliation when an active worker exists.
func (worker *Worker) Wake() { worker.execute("wake") }

// Stop prevents late readiness and waits for reconciliation to finish.
func (worker *Worker) Stop() { worker.execute("stop") }

func executeWorker(state *workerState, operation string) {
	if operation == "stop" {
		stopWorker(state)
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	updateWorker(state, operation)
}

func updateWorker(state *workerState, operation string) {
	switch operation {
	case "enable":
		state.enabled = true
	case "start":
		startWorker(state)
	case "wake":
		wakeWorker(state)
	}
}

func startWorker(state *workerState) {
	if !state.enabled || state.stopped || state.stop != nil {
		return
	}
	state.wake, state.stop = state.start()
}

func wakeWorker(state *workerState) {
	if state.wake != nil && !state.stopped {
		state.wake()
	}
}

func stopWorker(state *workerState) {
	state.mu.Lock()
	state.stopped = true
	stop := state.stop
	state.mu.Unlock()
	if stop != nil {
		stop()
	}
}

func acknowledgeDelivery(store ports.Store, pending []reviewapi.DueOccurrence) error {
	encoded, err := json.Marshal(pending)
	if err != nil {
		return fmt.Errorf("encode acknowledged occurrences: %w", err)
	}
	err = store.MarkNotified(string(encoded))
	if err != nil {
		return fmt.Errorf("acknowledge occurrences: %w", err)
	}
	return nil
}
