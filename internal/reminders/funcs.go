// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package reminders

import (
	"context"
	"fmt"
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
		if err != nil {
			return fmt.Errorf("deliver: %w", err)
		}

		return nil
	}

	sendErr := notifier.Send(len(pending))
	if sendErr != nil {
		return fmt.Errorf("deliver: %w", sendErr)
	}

	forwardErr := store.MarkNotified(pending)
	if forwardErr != nil {
		return fmt.Errorf("deliver: %w", forwardErr)
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
	coordinator.stopOnce.Do(coordinator.cancel)
	<-coordinator.done
}

func Start(store Source, notifier Notifier, publish func(), warn func(string, error)) *Coordinator {
	ctx, cancel := context.WithCancel(context.Background())
	coordinator := &Coordinator{
		cancel:  cancel,
		changed: make(chan struct{}, 1),
		done:    make(chan struct{}),
	}

	go func() {
		defer close(coordinator.done)

		if notifier != nil {
			prompt, err := store.ClaimNotificationPrompt()
			if err == nil && prompt {
				go func() {
					requestAuthorizationErr := notifier.RequestAuthorization()
					if requestAuthorizationErr != nil {
						warn("Notification authorization unavailable", requestAuthorizationErr)
					}

					coordinator.Wake()
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
					DeliverErr := Deliver(store, notifier, pending)
					if DeliverErr != nil {
						warn("Could not deliver review reminder", DeliverErr)
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
			case <-coordinator.changed:
				timer.Stop()
			case <-timer.C:
			}
		}
	}()

	return coordinator
}
