// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"encoding/json"
	"errors"
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
	items := contentapi.Items(card.Items)

	for index := range items {
		occurrence, next := itemSnapshot(card, &items[index], now)

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
	if item.Hint {
		return nil, pollInterval
	}

	due := contentapi.Schedule(item.Schedule).Due
	if due.After(now) {
		return nil, due.Sub(now)
	}

	dueMillis := due.UnixMilli()
	if item.LastNotifiedDue == dueMillis {
		return nil, pollInterval
	}

	return dueOccurrence(card, item, dueMillis), pollInterval
}

func dueOccurrence(
	card *cardapi.Card,
	item *contentapi.Item,
	dueMillis int64,
) *reviewapi.DueOccurrence {
	return &reviewapi.DueOccurrence{
		FolderID: card.FolderID,
		CardID:   card.ID,
		ItemID:   item.ID,
		Due:      dueMillis,
	}
}

// DecodeCards validates serialized persisted cards before schedule reconciliation.
func DecodeCards(data string) ([]cardapi.Card, error) {
	var cards []cardapi.Card

	err := contentapi.DecodeJSON([]byte(data), &cards)
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
	groups := reminderCards(pending)
	for index := range groups {
		err := deliverCard(store, notifier, groups[index])
		if err != nil {
			return fmt.Errorf("deliver card reminder: %w", err)
		}
	}

	return nil
}

func reminderCards(pending []reviewapi.DueOccurrence) [][]reviewapi.DueOccurrence {
	groups := [][]reviewapi.DueOccurrence{}
	indexes := map[string]int{}

	for position := range pending {
		key := fmt.Sprintf("%q/%q", pending[position].FolderID, pending[position].CardID)
		index, exists := indexes[key]

		if !exists {
			indexes[key], index = len(groups), len(groups)

			groups = append(groups, nil)
		}

		groups[index] = append(groups[index], pending[position])
	}

	return groups
}

func deliverCard(
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
	signals := new(coordinatorSignals)

	signals.stopping = make(chan struct{})
	signals.changed = make(chan struct{}, wakeBuffer)
	signals.done = make(chan struct{})

	runtime := newReconciliation(source, callbacks, signals)

	runtime.available = notificationAvailability(notifier)
	bindNotifications(runtime, source, notifier)

	go runReconciliation(runtime)

	return &Coordinator{wake: runtime.wake, stop: func() { stopCoordinator(signals) }}
}

func newReconciliation(
	source Source,
	callbacks Callbacks,
	signals *coordinatorSignals,
) *reconciliation {
	runtime := new(reconciliation)

	runtime.list, runtime.claim = source.ListCards, source.ClaimNotificationPrompt
	runtime.publish, runtime.warn = callbacks.Publish, callbacks.Warn
	runtime.wake = func() { wakeCoordinator(signals.changed) }
	runtime.wait = func(delay time.Duration) bool { return waitCoordinator(signals, delay) }
	runtime.finish = func() { close(signals.done) }

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

	return errors.Join(Deliver(source, notifier, pending))
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

	for {
		requestAuthorization(runtime)

		delay := reconcile(runtime)
		if !runtime.wait(delay) {
			return
		}
	}
}

func notificationAvailability(notifier ports.Notifier) func() bool {
	available, ok := notifier.(ports.Availability)
	if !ok {
		return nil
	}

	return available.Available
}

func notificationsAvailable(runtime *reconciliation) bool {
	return runtime.available == nil || runtime.available()
}

func authorizationPending(runtime *reconciliation) bool {
	return runtime.authorize != nil && !runtime.promptAttempted && notificationsAvailable(runtime)
}

func requestAuthorization(runtime *reconciliation) {
	if !authorizationPending(runtime) {
		return
	}

	runtime.promptAttempted = true

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

	cards, err := DecodeCards(encoded)

	return cards, errors.Join(err)
}

func deliverPending(runtime *reconciliation, pending []reviewapi.DueOccurrence) {
	if !notificationsAvailable(runtime) {
		return
	}

	if len(pending) == zero || runtime.deliver == nil {
		return
	}

	err := encodeDelivery(runtime.deliver, pending)
	if err != nil {
		runtime.warn("Could not deliver review reminder", err)
	}
}

func encodeDelivery(deliver func([]byte) error, pending any) error {
	encoded, err := marshalReminder(pending)
	if err != nil {
		return fmt.Errorf("encode due occurrences: %w", err)
	}

	return errors.Join(deliver(encoded))
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
	state := new(workerState)

	state.start = func() (func(), func()) {
		coordinator := startAvailable(source, notifications, callbacks)

		return coordinator.Wake, coordinator.Stop
	}

	worker := &Worker{execute: func(operation string) { executeWorker(state, operation) }}

	if changes, ok := notifications.(ports.AvailabilityChanges); ok {
		state.unsubscribe = changes.SubscribeAvailability(worker.Wake)
	}

	return worker
}

func startAvailable(source Source, notifications Notifications, callbacks Callbacks) *Coordinator {
	return Start(source, notifications, callbacks)
}

// Enable permits readiness events to start reconciliation.
func (worker *Worker) Enable() { worker.execute(enableCommand) }

// Start starts reconciliation once after lifecycle enablement.
func (worker *Worker) Start() { worker.execute(startCommand) }

// Wake requests reconciliation when an active worker exists.
func (worker *Worker) Wake() { worker.execute(wakeCommand) }

// Stop prevents late readiness and waits for reconciliation to finish.
func (worker *Worker) Stop() { worker.execute(stopCommand) }

func executeWorker(state *workerState, operation string) {
	if operation == stopCommand {
		stopWorker(state)

		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	updateWorker(state, operation)
}

func updateWorker(state *workerState, operation string) {
	switch operation {
	case enableCommand:
		state.enabled = true
	case startCommand:
		startWorker(state)
	case wakeCommand:
		wakeWorker(state)
	default:
		return
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
	unsubscribe := state.unsubscribe

	state.unsubscribe = nil
	state.mu.Unlock()

	if unsubscribe != nil {
		unsubscribe()
	}

	if stop != nil {
		stop()
	}
}

func acknowledgeDelivery(store ports.Store, pending any) error {
	encoded, err := marshalReminder(pending)
	if err != nil {
		return fmt.Errorf("encode acknowledged occurrences: %w", err)
	}

	err = store.MarkNotified(string(encoded))
	if err != nil {
		return fmt.Errorf("acknowledge occurrences: %w", err)
	}

	return nil
}

func marshalReminder(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode reminder value: %w", err)
	}

	return encoded, nil
}
