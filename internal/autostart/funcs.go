// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package autostart

import (
	"errors"
	"fmt"
	"slices"
)

func (c *Controller) Initialize(settings Settings) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.updateMenus()

	if c.debug || c.ready {
		return nil
	}

	first, err := settings.ClaimAutostartInitialization()
	if err != nil {
		return err
	}

	c.ready = true
	if err := c.readStatus(); err != nil {
		return err
	}

	if !first || c.enabled {
		return nil
	}

	return c.setEnabled(true)
}

func (c *Controller) Toggle() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.updateMenus()

	if c.debug || !c.ready {
		return nil
	}

	err := c.readStatus()
	if err != nil {
		return err
	}

	return c.setEnabled(!c.enabled)
}

func (c *Controller) Refresh() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.updateMenus()

	if c.debug || !c.ready {
		return nil
	}

	return c.readStatus()
}

func (c *Controller) setEnabled(enabled bool) error {
	var err error

	if enabled {
		err = c.backend.EnableWithOptions(Options{
			Identifier: ApplicationID,
			Arguments:  []string{autostartArgument},
		})
	} else {
		err = c.backend.Disable()
	}

	// Even a failed operation can change the OS state. Always read it back.
	statusErr := c.readStatus()

	if err != nil {
		err = fmt.Errorf("change login startup registration: %w", err)
	}

	if err == nil && statusErr == nil && enabled != c.enabled {
		err = errors.New(
			"login startup was not changed; check your operating system's startup settings and any required approval",
		)
	}

	return errors.Join(err, statusErr)
}

func (c *Controller) readStatus() error {
	enabled, err := c.backend.IsEnabled()
	if err != nil {
		return fmt.Errorf("read login startup registration: %w", err)
	}

	c.enabled = enabled

	return nil
}

func (c *Controller) updateMenus() {
	if c.publish != nil {
		c.publish(c.enabled, c.ready && !c.debug)
	}
}

func IsAutostartLaunch(args []string) bool {
	return slices.Contains(args, autostartArgument)
}

func (v *Visibility) ApplicationStarted() {
	v.change(func() { v.started = true })
}

func (v *Visibility) Classify(automatic bool) {
	v.change(func() {
		v.classified = true
		v.automatic = automatic
	})
}

func (v *Visibility) RequestOpen() {
	v.change(func() { v.open = true })
}

func (v *Visibility) SecondInstance(args []string) {
	if !IsAutostartLaunch(args) {
		v.RequestOpen()
	}
}

func (v *Visibility) change(change func()) {
	v.mu.Lock()
	change()

	show := false

	if v.started && v.classified {
		show = v.open || (!v.settled && !v.automatic)
		v.open = false
		v.settled = true
	}

	v.mu.Unlock()

	if show {
		v.show()
	}
}

func New(backend Backend, debug bool, publish func(bool, bool)) *Controller {
	return &Controller{backend: backend, debug: debug, publish: publish}
}

func NewVisibility(show func()) *Visibility { return &Visibility{show: show} }

func (c *Controller) SetPublisher(publish func(bool, bool)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.publish = publish
}
