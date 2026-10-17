// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"errors"
	"fmt"
	"slices"

	"raven/internal/features/autostart/ports"
)

// Initialize claims initial registration and reconciles observed startup state.
func (controller *Controller) Initialize() error { return controller.execute("initialize") }

// Toggle changes login registration using the current operating system state.
func (controller *Controller) Toggle() error { return controller.execute("toggle") }

// Refresh refreshes observed login registration.
func (controller *Controller) Refresh() error { return controller.execute("refresh") }

// SetPublisher installs the callback used to refresh login menus.
func (controller *Controller) SetPublisher(publish func(bool, bool)) {
	controller.publish(publish)
}

// New binds login registration to its required runtime dependencies.
func New(backend ports.Backend, settings ports.Settings, configuration *Configuration) *Controller {
	state := &registration{
		claim:   func() (bool, error) { return settings.ClaimAutostartInitialization() },
		status:  func() (bool, error) { return backend.IsEnabled() },
		enable:  registrationEnabler(backend, configuration.ApplicationID),
		disable: func() error { return backend.Disable() },
		debug:   configuration.Debug,
	}
	return &Controller{
		execute: registrationCommands(state),
		publish: func(publish func(bool, bool)) { setPublisher(state, publish) },
	}
}

func registrationEnabler(backend ports.Backend, identifier string) func() error {
	return func() error {
		return backend.EnableWithOptions(
			&ports.Options{Identifier: identifier, Arguments: []string{autostartArgument}},
		)
	}
}

func registrationCommands(state *registration) func(string) error {
	return func(operation string) error {
		return registrationCommand(state, operation)
	}
}

func registrationCommand(state *registration, operation string) error {
	switch operation {
	case "initialize":
		return initialize(state)
	case "toggle":
		return toggle(state)
	case "refresh":
		return refresh(state)
	default:
		return refresh(state)
	}
	return nil
}

func setPublisher(state *registration, publish func(bool, bool)) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.publish = publish
}

// Initialize claims initial registration and reconciles the observed startup state.
func initialize(controller *registration) error {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	defer updateMenus(controller)

	if controller.debug || controller.ready {
		return nil
	}

	err := initializeRegistration(controller)
	if err != nil {
		return fmt.Errorf("initialize login startup: %w", err)
	}

	return nil
}

func initializeRegistration(controller *registration) error {
	first, err := prepareRegistration(controller)
	if err != nil {
		return fmt.Errorf("prepare login startup: %w", err)
	}

	if !first || controller.enabled {
		return nil
	}

	err = setRegistration(controller, func() error { return enableRegistration(controller) })
	if err != nil {
		return fmt.Errorf("enable login startup on first launch: %w", err)
	}

	return nil
}

func prepareRegistration(controller *registration) (bool, error) {
	first, err := controller.claim()
	if err != nil {
		return false, fmt.Errorf("claim login startup initialization: %w", err)
	}

	controller.ready = true

	err = readStatus(controller)
	if err != nil {
		return false, fmt.Errorf("initialize login startup status: %w", err)
	}

	return first, nil
}

// Toggle changes login startup registration using the current OS state.
func toggle(controller *registration) error {
	err := whenReady(controller, func() error { return toggleRegistration(controller) })
	if err != nil {
		return fmt.Errorf("toggle login startup: %w", err)
	}

	return nil
}

func toggleRegistration(controller *registration) error {
	err := readStatus(controller)
	if err != nil {
		return fmt.Errorf("refresh before toggling login startup: %w", err)
	}

	operation := func() error { return enableRegistration(controller) }
	if controller.enabled {
		operation = func() error { return disableRegistration(controller) }
	}

	err = setRegistration(controller, operation)
	if err != nil {
		return fmt.Errorf("change login startup state: %w", err)
	}

	return nil
}

// Refresh refreshes the observed login startup state.
func refresh(controller *registration) error {
	err := whenReady(controller, func() error { return readStatus(controller) })
	if err != nil {
		return fmt.Errorf("refresh login startup: %w", err)
	}

	return nil
}

func whenReady(controller *registration, operation func() error) error {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	defer updateMenus(controller)

	if controller.debug || !controller.ready {
		return nil
	}

	err := operation()
	if err != nil {
		return fmt.Errorf("update login startup state: %w", err)
	}

	return nil
}

func setRegistration(controller *registration, operation func() error) error {
	expected := !controller.enabled
	changed := operation()
	// Even a failed operation can change the OS state. Always read it back.
	observed := readStatus(controller)
	if changed != nil || observed != nil {
		return errors.Join(changed, observed)
	}

	if expected != controller.enabled {
		return errLoginStartupNotChanged
	}

	return nil
}

func disableRegistration(controller *registration) error {
	err := controller.disable()
	if err != nil {
		return fmt.Errorf("disable login startup registration: %w", err)
	}

	return nil
}

func enableRegistration(controller *registration) error {
	err := controller.enable()
	if err != nil {
		return fmt.Errorf("enable login startup registration: %w", err)
	}

	return nil
}

func readStatus(controller *registration) error {
	enabled, err := controller.status()
	if err != nil {
		return fmt.Errorf("read login startup registration: %w", err)
	}

	controller.enabled = enabled

	return nil
}

func updateMenus(controller *registration) {
	if controller.publish != nil {
		controller.publish(controller.enabled, controller.ready && !controller.debug)
	}
}

// IsAutostartLaunch reports whether arguments request automatic startup.
func IsAutostartLaunch(args []string) bool { return slices.Contains(args, autostartArgument) }

// ApplicationStarted records readiness of the native application.
func (visibility *Visibility) ApplicationStarted() { visibility.execute("started", false) }

// Classify records the operating system's launch classification.
func (visibility *Visibility) Classify(automatic bool) { visibility.execute("classify", automatic) }

// RequestOpen records an explicit request to show the main window.
func (visibility *Visibility) RequestOpen() { visibility.execute("open", false) }

// SecondInstance handles an additional application launch.
func (visibility *Visibility) SecondInstance(args []string) {
	if !IsAutostartLaunch(args) {
		visibility.execute("open", false)
	}
}

// Pending returns coalesced requests to show the main window.
func (visibility *Visibility) Pending() <-chan struct{} { return visibility.execute("pending", false) }

// Visibility returns the launch policy associated with the request queue.
func (visibility *Visibility) Visibility() *Visibility { return visibility }

// NewVisibility creates a launch visibility coordinator with an opening callback.
func NewVisibility(show func()) *Visibility {
	state := &launchState{show: show}
	return &Visibility{execute: launchCommands(state)}
}

// NewRequests creates a queue that is safe before native windows exist.
func NewRequests() *Visibility {
	state := &launchState{pending: make(chan struct{}, requestBuffer)}
	state.show = func() { enqueueOpen(state.pending) }
	return &Visibility{execute: launchCommands(state)}
}

func enqueueOpen(pending chan struct{}) {
	select {
	case pending <- struct{}{}:
	default:
	}
}

func launchCommands(state *launchState) func(string, bool) <-chan struct{} {
	return func(operation string, automatic bool) <-chan struct{} {
		changeLaunch(state, operation, automatic)
		return state.pending
	}
}

func changeLaunch(state *launchState, operation string, automatic bool) {
	state.mu.Lock()
	updateLaunch(state, operation, automatic)
	show := takeOpenRequest(state)
	state.mu.Unlock()
	if show {
		state.show()
	}
}

func updateLaunch(state *launchState, operation string, automatic bool) {
	switch operation {
	case "started":
		state.started = true
	case "classify":
		state.classified = true
		state.automatic = automatic
	case "open":
		state.open = true
	}
}

func takeOpenRequest(state *launchState) bool {
	if !state.started || !state.classified {
		return false
	}
	show := state.open || (!state.settled && !state.automatic)
	state.open = false
	state.settled = true
	return show
}

// PrepareLaunch registers native classification before window construction.
func PrepareLaunch(detector ports.LoginDetection, visibility LaunchClassification, args []string) {
	automaticArgument := IsAutostartLaunch(args)
	detector.PrepareLoginDetection(func(automatic bool) {
		go visibility.Classify(automatic || automaticArgument)
	})
}
