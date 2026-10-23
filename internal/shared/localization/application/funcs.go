// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"fmt"
	"sync"

	"raven/internal/shared/localization/domain"
	"raven/internal/shared/localization/ports"
)

// New allocates language state without acquiring resources.
func New(preferences ports.Preferences, system ports.SystemLocales, warn func(error)) *Service {
	runtime := &state{
		change:      sync.Mutex{},
		mu:          sync.RWMutex{},
		preferences: preferences,
		system:      system,
		warn:        warn,
		current:     domain.Resolve(domain.System, nil),
		listeners:   make(map[uint64]func()),
		next:        initialRevision,
	}

	return &Service{operations: operations(runtime)}
}

func operations(runtime *state) map[string]func(string, func()) result {
	return map[string]func(string, func()) result{
		initializeOperation: initialization(runtime),
		currentOperation:    snapshotOperation(runtime),
		setOperation:        preferenceOperation(runtime),
		refreshOperation:    systemOperation(runtime),
		subscribeOperation:  observationOperation(runtime),
	}
}

func initialization(runtime *state) func(string, func()) result {
	return func(string, func()) result {
		initialize(runtime)

		return emptyResult()
	}
}

func snapshotOperation(runtime *state) func(string, func()) result {
	return func(string, func()) result {
		outcome := emptyResult()

		outcome.state = current(runtime)

		return outcome
	}
}

func preferenceOperation(runtime *state) func(string, func()) result {
	return func(preference string, _ func()) result {
		outcome := emptyResult()

		outcome.state, outcome.failure = set(runtime, preference)

		return outcome
	}
}

func systemOperation(runtime *state) func(string, func()) result {
	return func(string, func()) result {
		refresh(runtime)

		return emptyResult()
	}
}

func observationOperation(runtime *state) func(string, func()) result {
	return func(_ string, listener func()) result {
		outcome := emptyResult()

		outcome.cancel = subscribe(runtime, listener)

		return outcome
	}
}

func emptyResult() result {
	return result{state: domain.Resolve(domain.System, nil), failure: nil, cancel: nil}
}

// Initialize loads preferences after persistence has started.
func (service *Service) Initialize() { service.operations[initializeOperation]("", nil) }

// Current returns an owned immutable snapshot.
func (service *Service) Current() domain.State {
	return service.operations[currentOperation]("", nil).state
}

// Set persists a validated choice before publishing it.
func (service *Service) Set(preference string) (domain.State, error) {
	outcome := service.operations[setOperation](preference, nil)

	return outcome.state, outcome.failure
}

// Refresh follows OS language changes while System is selected.
func (service *Service) Refresh() { service.operations[refreshOperation]("", nil) }

// Subscribe observes committed changes until the returned function is called.
func (service *Service) Subscribe(listener func()) func() {
	return service.operations[subscribeOperation]("", listener).cancel
}

func current(runtime *state) domain.State {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()

	return runtime.current
}

func initialize(runtime *state) {
	runtime.change.Lock()
	defer runtime.change.Unlock()

	preference, err := runtime.preferences.LoadLanguagePreference()
	if err != nil {
		runtime.warn(err)
	}

	publish(runtime, resolve(runtime, preference))
}

func set(runtime *state, preference string) (domain.State, error) {
	runtime.change.Lock()
	defer runtime.change.Unlock()

	if !domain.ValidPreference(preference) {
		return current(runtime), ErrInvalidPreference
	}

	err := runtime.preferences.SaveLanguagePreference(preference)
	if err != nil {
		runtime.warn(err)

		return current(runtime), fmt.Errorf("save language preference: %w", err)
	}

	publish(runtime, resolve(runtime, preference))

	return current(runtime), nil
}

func refresh(runtime *state) {
	runtime.change.Lock()
	defer runtime.change.Unlock()

	if current(runtime).Preference == domain.System {
		publish(runtime, resolve(runtime, domain.System))
	}
}

func resolve(runtime *state, preference string) *domain.State {
	resolved := domain.Resolve(preference, systemLanguages(runtime, preference))

	return &resolved
}

func systemLanguages(runtime *state, preference string) []string {
	if preference != domain.System && domain.ValidPreference(preference) {
		return nil
	}

	locales, err := runtime.system.Locales()
	if err != nil {
		runtime.warn(err)
	}

	return locales
}

func publish(runtime *state, resolved *domain.State) {
	listeners := commit(runtime, resolved)
	for index := range listeners {
		listeners[index]()
	}
}

func commit(runtime *state, resolved *domain.State) []func() {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	previous := runtime.current
	if unchanged(&previous, resolved) {
		return nil
	}

	resolved.Revision = previous.Revision + nextRevision
	runtime.current = *resolved

	return listenersSnapshot(runtime.listeners)
}

func unchanged(previous, resolved *domain.State) bool {
	return previous.Revision > initialRevision && previous.Preference == resolved.Preference &&
		previous.Locale == resolved.Locale
}

func listenersSnapshot(listeners map[uint64]func()) []func() {
	snapshot := make([]func(), initialRevision, len(listeners))
	for id := range listeners {
		snapshot = append(snapshot, listeners[id])
	}

	return snapshot
}

func subscribe(runtime *state, listener func()) func() {
	runtime.mu.Lock()

	runtime.next++

	id := runtime.next

	runtime.listeners[id] = listener
	runtime.mu.Unlock()

	return func() { runtime.mu.Lock(); delete(runtime.listeners, id); runtime.mu.Unlock() }
}
