// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package reminders

import (
	"context"
	"time"

	"raven/internal/library"
)

func Snapshot(
	cards []library.Card,
	now time.Time,
) ([]library.DueOccurrence, time.Duration) {
	pending := []library.DueOccurrence{}
	delay := 30 * time.Second

	for _, card := range cards {
		for _, item := range card.Items {
			due := item.Schedule.Due
			if !due.After(now) {
				if item.LastNotifiedDue != due.UnixMilli() {
					pending = append(
						pending,
						library.DueOccurrence{
							FolderID: card.FolderID,
							CardID:   card.ID,
							ItemID:   item.ID,
							Due:      due.UnixMilli(),
						},
					)
				}
			} else if until := due.Sub(now); until < delay {
				delay = until
			}
		}
	}

	return pending, delay
}

func Deliver(
	store Store,
	notifier Notifier,
	pending []library.DueOccurrence,
) error {
	allowed, err := notifier.CheckNotificationAuthorization()
	if err != nil || !allowed {
		return err
	}

	if err := notifier.Send(len(pending)); err != nil {
		return err
	}

	return store.MarkNotified(pending)
}

func (c *Coordinator) Wake() {
	select {
	case c.changed <- struct{}{}:
	default:
	}
}
func (c *Coordinator) Stop() { c.stopOnce.Do(c.cancel); <-c.done }
func Start(store Source, notifier Notifier, publish func(), warn func(string, error)) *Coordinator {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Coordinator{cancel: cancel, changed: make(chan struct{}, 1), done: make(chan struct{})}

	go func() {
		defer close(c.done)

		if notifier != nil {
			prompt, err := store.ClaimNotificationPrompt()
			if err == nil && prompt {
				go func() {
					err := notifier.RequestAuthorization()
					if err != nil {
						warn("Notification authorization unavailable", err)
					}

					c.Wake()
				}()
			}
		}

		for {
			delay := 30 * time.Second

			cards, err := store.ListCards()
			if err == nil {
				pending, next := Snapshot(cards, time.Now())

				delay = next

				publish()

				if len(pending) > 0 && notifier != nil {
					err := Deliver(store, notifier, pending)
					if err != nil {
						warn("Could not deliver review reminder", err)
					}
				}
			} else {
				warn("Could not read review schedules", err)
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

	return c
}
