// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"errors"
	"fmt"
	"slices"
	"sync"
)

// Initialize claims initial registration and reconciles the observed startup state.
func (controller *Controller) Initialize(settings Settings) error {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	defer controller.updateMenus()

	if controller.debug || controller.ready {
		return nil
	}

	err := controller.initializeRegistration(settings)
	if err != nil {
		return fmt.Errorf("initialize login startup: %w", err)
	}

	return nil
}

func (controller *Controller) initializeRegistration(settings Settings) error {
	first, err := controller.prepareRegistration(settings)
	if err != nil {
		return fmt.Errorf("prepare login startup: %w", err)
	}

	if !first || controller.enabled {
		return nil
	}

	err = controller.setRegistration(controller.enableRegistration)
	if err != nil {
		return fmt.Errorf("enable login startup on first launch: %w", err)
	}

	return nil
}

func (controller *Controller) prepareRegistration(settings Settings) (bool, error) {
	first, err := settings.ClaimAutostartInitialization()
	if err != nil {
		return false, fmt.Errorf("claim login startup initialization: %w", err)
	}

	controller.ready = true

	err = controller.readStatus()
	if err != nil {
		return false, fmt.Errorf("initialize login startup status: %w", err)
	}

	return first, nil
}

// Toggle changes login startup registration using the current OS state.
func (controller *Controller) Toggle() error {
	err := controller.whenReady(controller.toggleRegistration)
	if err != nil {
		return fmt.Errorf("toggle login startup: %w", err)
	}

	return nil
}

func (controller *Controller) toggleRegistration() error {
	err := controller.readStatus()
	if err != nil {
		return fmt.Errorf("refresh before toggling login startup: %w", err)
	}

	operation := controller.enableRegistration
	if controller.enabled {
		operation = controller.disableRegistration
	}

	err = controller.setRegistration(operation)
	if err != nil {
		return fmt.Errorf("change login startup state: %w", err)
	}

	return nil
}

// Refresh refreshes the observed login startup state.
func (controller *Controller) Refresh() error {
	err := controller.whenReady(controller.readStatus)
	if err != nil {
		return fmt.Errorf("refresh login startup: %w", err)
	}

	return nil
}

func (controller *Controller) whenReady(operation func() error) error {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	defer controller.updateMenus()

	if controller.debug || !controller.ready {
		return nil
	}

	err := operation()
	if err != nil {
		return fmt.Errorf("update login startup state: %w", err)
	}

	return nil
}

func (controller *Controller) setRegistration(operation func() error) error {
	expected := !controller.enabled
	changed := operation()
	// Even a failed operation can change the OS state. Always read it back.
	observed := controller.readStatus()
	if changed != nil || observed != nil {
		return errors.Join(changed, observed)
	}

	if expected != controller.enabled {
		return errLoginStartupNotChanged
	}

	return nil
}

func (controller *Controller) disableRegistration() error {
	err := controller.backend.Disable()
	if err != nil {
		return fmt.Errorf("disable login startup registration: %w", err)
	}

	return nil
}

func (controller *Controller) enableRegistration() error {
	err := controller.backend.EnableWithOptions(&Options{
		Identifier: controller.applicationID,
		Arguments:  []string{autostartArgument},
	})
	if err != nil {
		return fmt.Errorf("enable login startup registration: %w", err)
	}

	return nil
}

func (controller *Controller) readStatus() error {
	enabled, err := controller.backend.IsEnabled()
	if err != nil {
		return fmt.Errorf("read login startup registration: %w", err)
	}

	controller.enabled = enabled

	return nil
}

func (controller *Controller) updateMenus() {
	if controller.publish != nil {
		controller.publish(controller.enabled, controller.ready && !controller.debug)
	}
}

// IsAutostartLaunch reports whether launch arguments request automatic startup.
func IsAutostartLaunch(args []string) bool { return slices.Contains(args, autostartArgument) }

// ApplicationStarted records that the native application has started.
func (visibility *Visibility) ApplicationStarted() {
	visibility.change(func() { visibility.started = true })
}

// Classify records whether the launch was automatic.
func (visibility *Visibility) Classify(automatic bool) {
	visibility.change(func() { visibility.classified = true; visibility.automatic = automatic })
}

// RequestOpen records an explicit request to show the main window.
func (visibility *Visibility) RequestOpen() { visibility.change(func() { visibility.open = true }) }

// SecondInstance handles an additional application launch.
func (visibility *Visibility) SecondInstance(args []string) {
	if !IsAutostartLaunch(args) {
		visibility.RequestOpen()
	}
}

func (visibility *Visibility) change(change func()) {
	visibility.mu.Lock()
	change()

	show := visibility.takeOpenRequest()
	visibility.mu.Unlock()

	if show {
		visibility.show()
	}
}

func (visibility *Visibility) takeOpenRequest() bool {
	if !visibility.started || !visibility.classified {
		return false
	}

	show := visibility.open || visibility.firstManualLaunch()

	visibility.open = false
	visibility.settled = true

	return show
}

func (visibility *Visibility) firstManualLaunch() bool {
	return !visibility.settled && !visibility.automatic
}

// New constructs the adapter with its required runtime dependencies.
func New(backend Backend, debug bool, applicationID string) *Controller {
	return &Controller{
		backend:       backend,
		applicationID: applicationID,
		debug:         debug,
		publish:       nil,
		mu:            sync.Mutex{},
		ready:         false,
		enabled:       false,
	}
}

// NewVisibility creates a launch visibility coordinator.
func NewVisibility(show func()) *Visibility {
	return &Visibility{
		show:       show,
		mu:         sync.Mutex{},
		started:    false,
		classified: false,
		automatic:  false,
		open:       false,
		settled:    false,
	}
}

// SetPublisher sets the callback used to update login startup menus.
func (controller *Controller) SetPublisher(publish func(bool, bool)) {
	controller.mu.Lock()
	defer controller.mu.Unlock()

	controller.publish = publish
}
