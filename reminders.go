package main

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"raven/internal/storage"
)

type reminderCoordinator struct {
	cancel   context.CancelFunc
	changed  chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

func (c *reminderCoordinator) wake() {
	select {
	case c.changed <- struct{}{}:
	default:
	}
}
func (c *reminderCoordinator) stop() { c.stopOnce.Do(c.cancel); <-c.done }

// A bounded reconciliation interval handles clock changes and resume from sleep.
func reminderSnapshot(cards []storage.Card, now time.Time) ([]storage.DueOccurrence, time.Duration) {
	pending := []storage.DueOccurrence{}
	delay := 30 * time.Second
	for _, card := range cards {
		for _, item := range card.Items {
			due := item.Schedule.Due
			if !due.After(now) {
				if item.LastNotifiedDue != due.UnixMilli() {
					pending = append(pending, storage.DueOccurrence{FolderID: card.FolderID, CardID: card.ID, ItemID: item.ID, Due: due.UnixMilli()})
				}
			} else if until := due.Sub(now); until < delay {
				delay = until
			}
		}
	}
	return pending, delay
}

func (r *RavenService) startReminders() {
	r.reminderMu.Lock()
	defer r.reminderMu.Unlock()
	if r.store == nil || r.reminders != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &reminderCoordinator{cancel: cancel, changed: make(chan struct{}, 1), done: make(chan struct{})}
	r.reminders = c
	go func() {
		defer close(c.done)
		// The OS authorization dialog must not block badge updates or shutdown.
		if r.notifier != nil {
			prompt, err := r.store.ClaimNotificationPrompt()
			if err == nil && prompt {
				go func() {
					if _, err := r.notifier.RequestNotificationAuthorization(); err != nil {
						r.app.Logger.Warn("Notification authorization unavailable", "error", err)
					}
					c.wake()
				}()
			}
		}

		for {
			delay := 30 * time.Second
			cards, err := r.store.ListCards()
			if err == nil {
				pending, next := reminderSnapshot(cards, time.Now())
				delay = next
				r.app.Event.Emit("raven:schedules-changed")
				if len(pending) > 0 && r.notifier != nil {
					if err := deliverReminders(r.store, r.notifier, pending); err != nil {
						r.app.Logger.Warn("Could not deliver review reminder", "error", err)
					}
				}
			} else {
				r.app.Logger.Warn("Could not read review schedules", "error", err)
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-c.changed:
				timer.Stop()
			case <-timer.C:
			}
		}
	}()
}

type reminderNotifier interface {
	CheckNotificationAuthorization() (bool, error)
	SendNotification(notifications.NotificationOptions) error
}

func deliverReminders(store *storage.Store, notifier reminderNotifier, pending []storage.DueOccurrence) error {
	allowed, err := notifier.CheckNotificationAuthorization()
	if err != nil || !allowed {
		return err
	}
	var sound *notifications.NotificationSound
	if runtime.GOOS == "darwin" {
		sound = &notifications.NotificationSound{Name: "raven.wav"}
	}
	if err := notifier.SendNotification(notifications.NotificationOptions{
		Sound: sound,
		ID: "raven-due", Title: "Time to review", Body: fmt.Sprintf("%d items are ready to review in Raven.", len(pending)), ThreadID: "raven-reviews",
	}); err != nil {
		return err
	}
	return store.MarkNotified(pending)
}
