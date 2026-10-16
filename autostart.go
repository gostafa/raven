package main

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const autostartArgument = "--autostart"
const applicationID = "app.raven.desktop"

type autostartBackend interface {
	EnableWithOptions(application.AutostartOptions) error
	Disable() error
	IsEnabled() (bool, error)
}

type autostartSettings interface {
	ClaimAutostartInitialization() (bool, error)
}

// All OS operations and menu publications share this lock, including refreshes.
// Read OS state before toggling: a checkbox may be stale after external changes.
type autostartController struct {
	mu      sync.Mutex
	backend autostartBackend
	debug   bool
	ready   bool
	enabled bool
	publish func(checked, available bool)
}

func (c *autostartController) initialize(settings autostartSettings) error {
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

func (c *autostartController) toggle() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.updateMenus()
	if c.debug || !c.ready {
		return nil
	}
	if err := c.readStatus(); err != nil {
		return err
	}
	return c.setEnabled(!c.enabled)
}

func (c *autostartController) refresh() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.updateMenus()
	if c.debug || !c.ready {
		return nil
	}
	return c.readStatus()
}

func (c *autostartController) setEnabled(enabled bool) error {
	var err error
	if enabled {
		err = c.backend.EnableWithOptions(application.AutostartOptions{
			Identifier: applicationID,
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
		err = errors.New("login startup was not changed; check your operating system's startup settings and any required approval")
	}
	return errors.Join(err, statusErr)
}

func (c *autostartController) readStatus() error {
	enabled, err := c.backend.IsEnabled()
	if err != nil {
		return fmt.Errorf("read login startup registration: %w", err)
	}
	c.enabled = enabled
	return nil
}

func (c *autostartController) updateMenus() {
	if c.publish != nil {
		c.publish(c.enabled, c.ready && !c.debug)
	}
}

func isAutostartLaunch(args []string) bool {
	return slices.Contains(args, autostartArgument)
}

// Native launch classification and ApplicationStarted may arrive in either order.
// Explicit open requests arriving during startup are retained until both are ready.
type launchVisibility struct {
	mu         sync.Mutex
	started    bool
	classified bool
	automatic  bool
	open       bool
	settled    bool
	show       func()
}

func (v *launchVisibility) applicationStarted() {
	v.change(func() { v.started = true })
}

func (v *launchVisibility) classify(automatic bool) {
	v.change(func() {
		v.classified = true
		v.automatic = automatic
	})
}

func (v *launchVisibility) requestOpen() {
	v.change(func() { v.open = true })
}

func (v *launchVisibility) secondInstance(args []string) {
	if !isAutostartLaunch(args) {
		v.requestOpen()
	}
}

func (v *launchVisibility) change(change func()) {
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
