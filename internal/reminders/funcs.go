// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package reminders

import (
	"fmt"
	"sync"
	"time"

	"raven/internal/library"
)

// Snapshot returns unacknowledged due items and the next reminder delay.
func Snapshot(cards []library.Card, now time.Time) ([]library.DueOccurrence, time.Duration) {
	snapshot := reminderSnapshot{pending: []library.DueOccurrence{}, delay: pollInterval, now: now}

	for cardIndex := range cards {
		card := &cards[cardIndex]
		for itemIndex := range card.Items {
			snapshot.add(card, &card.Items[itemIndex])
		}
	}

	return snapshot.pending, snapshot.delay
}

func (snapshot *reminderSnapshot) add(card *library.Card, item *library.ContentItem) {
	due := item.Schedule.Due
	if due.After(snapshot.now) {
		snapshot.delay = min(snapshot.delay, due.Sub(snapshot.now))

		return
	}

	dueMillis := due.UnixMilli()
	if item.LastNotifiedDue == dueMillis {
		return
	}

	occurrence := library.DueOccurrence{
		FolderID: card.FolderID, CardID: card.ID, ItemID: item.ID, Due: dueMillis,
	}

	snapshot.pending = append(snapshot.pending, occurrence)
}

// Deliver sends an authorized reminder before acknowledging unchanged schedules.
func Deliver(store Store, notifier Notifier, pending []library.DueOccurrence) error {
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

func deliverAuthorized(store Store, notifier Notifier, pending []library.DueOccurrence) error {
	err := notifier.Send(len(pending))
	if err != nil {
		return fmt.Errorf("send review reminder: %w", err)
	}

	err = store.MarkNotified(pending)
	if err != nil {
		return fmt.Errorf("acknowledge review reminder: %w", err)
	}

	return nil
}

// Wake requests an immediate reminder reconciliation.
func (coordinator *Coordinator) Wake() {
	select {
	case coordinator.changed <- struct{}{}:
	default:
	}
}

// Stop cancels reminder reconciliation and waits for it to finish.
func (coordinator *Coordinator) Stop() {
	coordinator.stopOnce.Do(func() { close(coordinator.stopping) })
	<-coordinator.done
}

// Start begins reminder reconciliation with the supplied effects.
func Start(store Source, notifier Notifier, callbacks Callbacks) *Coordinator {
	stopping := make(chan struct{})
	coordinator := &Coordinator{
		source:    store,
		notifier:  notifier,
		callbacks: callbacks,
		stopping:  stopping,
		changed:   make(chan struct{}, wakeBuffer),
		done:      make(chan struct{}),
		stopOnce:  sync.Once{},
	}

	go coordinator.run()

	return coordinator
}

func (coordinator *Coordinator) run() {
	defer close(coordinator.done)

	coordinator.requestAuthorization()

	for {
		delay := coordinator.reconcile()
		if !coordinator.wait(delay) {
			return
		}
	}
}

func (coordinator *Coordinator) requestAuthorization() {
	if coordinator.notifier == nil {
		return
	}

	prompt, err := coordinator.source.ClaimNotificationPrompt()
	if err != nil || !prompt {
		return
	}

	go coordinator.authorize()
}

func (coordinator *Coordinator) authorize() {
	err := coordinator.notifier.RequestAuthorization()
	if err != nil {
		coordinator.callbacks.Warn("Notification authorization unavailable", err)
	}

	coordinator.Wake()
}

func (coordinator *Coordinator) reconcile() time.Duration {
	cards, err := coordinator.source.ListCards()
	if err != nil {
		coordinator.callbacks.Warn("Could not read review schedules", err)

		return pollInterval
	}

	pending, delay := Snapshot(cards, time.Now())

	coordinator.callbacks.Publish()
	coordinator.deliver(pending)

	return delay
}

func (coordinator *Coordinator) deliver(pending []library.DueOccurrence) {
	if len(pending) == zero || coordinator.notifier == nil {
		return
	}

	err := Deliver(coordinator.source, coordinator.notifier, pending)
	if err != nil {
		coordinator.callbacks.Warn("Could not deliver review reminder", err)
	}
}

func (coordinator *Coordinator) wait(delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-coordinator.stopping:
		return false
	case <-coordinator.changed:
		return true
	case <-timer.C:
		return true
	}
}
