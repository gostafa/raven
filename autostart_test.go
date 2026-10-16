package main

import (
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeAutostart struct {
	enabled                  bool
	enables, disables, reads int
	options                  application.AutostartOptions
	operationErr, statusErr  error
	ignoreEnable             bool
}

func (f *fakeAutostart) EnableWithOptions(options application.AutostartOptions) error {
	f.enables++
	f.options = options
	if f.operationErr == nil && !f.ignoreEnable {
		f.enabled = true
	}
	return f.operationErr
}
func (f *fakeAutostart) Disable() error {
	f.disables++
	if f.operationErr == nil {
		f.enabled = false
	}
	return f.operationErr
}
func (f *fakeAutostart) IsEnabled() (bool, error) {
	f.reads++
	return f.enabled, f.statusErr
}

type fakeAutostartSettings struct {
	claimed bool
	calls   int
	err     error
}

func (s *fakeAutostartSettings) ClaimAutostartInitialization() (bool, error) {
	s.calls++
	if s.err != nil {
		return false, s.err
	}
	first := !s.claimed
	s.claimed = true
	return first, nil
}

func TestAutostartDefaultsAndMenuState(t *testing.T) {
	for _, tc := range []struct {
		name                                                string
		debug, claimed, enabled, wantChecked, wantAvailable bool
		wantEnables                                         int
	}{
		{"first production launch", false, false, false, true, true, 1},
		{"existing registration", false, false, true, true, true, 0},
		{"previously disabled", false, true, false, false, true, 0},
		{"previously enabled", false, true, true, true, true, 0},
		{"development", true, false, false, false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := &fakeAutostart{enabled: tc.enabled}
			settings := &fakeAutostartSettings{claimed: tc.claimed}
			checked, available := false, false
			c := &autostartController{backend: backend, debug: tc.debug, publish: func(b, a bool) { checked, available = b, a }}
			if err := c.initialize(settings); err != nil {
				t.Fatal(err)
			}
			if checked != tc.wantChecked || available != tc.wantAvailable || backend.enables != tc.wantEnables {
				t.Fatalf("checked=%v available=%v enables=%d", checked, available, backend.enables)
			}
			if tc.debug {
				_ = c.toggle()
				_ = c.refresh()
				if settings.calls != 0 || backend.reads != 0 || backend.enables != 0 || backend.disables != 0 {
					t.Fatal("development touched OS settings")
				}
			}
			if tc.wantEnables > 0 && !reflect.DeepEqual(backend.options, application.AutostartOptions{Identifier: applicationID, Arguments: []string{autostartArgument}}) {
				t.Fatalf("registration: %+v", backend.options)
			}
		})
	}
}

func TestAutostartTogglesAndExternalChangesSurviveRestart(t *testing.T) {
	backend, settings := &fakeAutostart{}, &fakeAutostartSettings{}
	checked := false
	newController := func() *autostartController {
		return &autostartController{backend: backend, publish: func(b, _ bool) { checked = b }}
	}
	c := newController()
	if err := c.initialize(settings); err != nil {
		t.Fatal(err)
	}
	if err := c.toggle(); err != nil || checked || backend.enabled {
		t.Fatalf("disable failed: %v", err)
	}
	c = newController()
	if err := c.initialize(settings); err != nil || checked || backend.enables != 1 {
		t.Fatalf("restart undid opt-out: %v", err)
	}
	if err := c.toggle(); err != nil || !checked {
		t.Fatalf("re-enable failed: %v", err)
	}
	backend.enabled = false
	if err := c.refresh(); err != nil || checked || backend.enables != 2 {
		t.Fatalf("refresh undid OS disable: %v", err)
	}
	backend.enabled = true // Checkbox is now stale.
	if err := c.toggle(); err != nil || checked || backend.enabled {
		t.Fatalf("toggle used stale state: %v", err)
	}
}

func TestAutostartFailuresAndExplicitRetry(t *testing.T) {
	failure := errors.New("permission denied")
	backend, settings := &fakeAutostart{operationErr: failure}, &fakeAutostartSettings{}
	c := &autostartController{backend: backend}
	if err := c.initialize(settings); !errors.Is(err, failure) || !settings.claimed || c.enabled {
		t.Fatalf("initial failure: %v", err)
	}
	backend.operationErr = nil
	c = &autostartController{backend: backend}
	if err := c.initialize(settings); err != nil || backend.enables != 1 {
		t.Fatalf("default retried automatically: %v", err)
	}
	if err := c.toggle(); err != nil || !backend.enabled {
		t.Fatalf("explicit retry: %v", err)
	}
	backend.operationErr = failure
	if err := c.toggle(); !errors.Is(err, failure) || !c.enabled {
		t.Fatalf("failed disable lost actual state: %v", err)
	}
	backend.statusErr = failure
	before := backend.disables
	if err := c.toggle(); !errors.Is(err, failure) || backend.disables != before {
		t.Fatalf("mutated after status error: %v", err)
	}
}

func TestAutostartRequiresDurableMarkerAndEffectiveRegistration(t *testing.T) {
	failure := errors.New("database closed")
	backend := &fakeAutostart{}
	c := &autostartController{backend: backend}
	if err := c.initialize(&fakeAutostartSettings{err: failure}); !errors.Is(err, failure) || c.ready || backend.reads != 0 {
		t.Fatalf("marker failure: %v", err)
	}
	_ = c.toggle()
	if backend.enables != 0 {
		t.Fatal("toggle bypassed failed marker")
	}
	c = &autostartController{backend: &fakeAutostart{ignoreEnable: true}}
	if err := c.initialize(&fakeAutostartSettings{}); err == nil || c.enabled {
		t.Fatal("reported success while OS approval still required")
	}
}

func TestAutostartSerializesMenuActions(t *testing.T) {
	backend := &fakeAutostart{}
	c := &autostartController{backend: backend}
	if err := c.initialize(&fakeAutostartSettings{}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if err := c.toggle(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if !backend.enabled || backend.enables != 11 || backend.disables != 10 {
		t.Fatalf("concurrent toggles: %+v", backend)
	}
}

func TestLaunchVisibilityWaitsForBothStartupEvents(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		for _, nativeFirst := range []bool{false, true} {
			shows := 0
			v := &launchVisibility{show: func() { shows++ }}
			if nativeFirst {
				v.classify(automatic)
			} else {
				v.applicationStarted()
			}
			if shows != 0 {
				t.Fatal("main opened before startup completed")
			}
			if nativeFirst {
				v.applicationStarted()
			} else {
				v.classify(automatic)
			}
			want := 1
			if automatic {
				want = 0
			}
			v.applicationStarted()
			if shows != want {
				t.Fatalf("automatic=%v nativeFirst=%v shows=%d", automatic, nativeFirst, shows)
			}
		}
	}
}

func TestSecondInstanceVisibility(t *testing.T) {
	for _, early := range []bool{false, true} {
		shows := 0
		v := &launchVisibility{show: func() { shows++ }}
		if early {
			v.secondInstance([]string{"raven"})
		}
		v.classify(true)
		v.applicationStarted()
		want := 0
		if early {
			want = 1
		}
		v.secondInstance([]string{"raven", autostartArgument})
		if shows != want {
			t.Fatal("automatic second launch opened main")
		}
		v.secondInstance([]string{"raven"})
		if shows != want+1 {
			t.Fatal("normal second launch did not open main")
		}
	}
	for _, arg := range []string{"", "--autostart=false", "file--autostart"} {
		if isAutostartLaunch([]string{arg}) {
			t.Fatalf("misclassified argument %q", arg)
		}
	}
}

func TestLaunchVisibilityConcurrentStartup(t *testing.T) {
	var shows atomic.Int32
	v := &launchVisibility{show: func() { shows.Add(1) }}
	var wg sync.WaitGroup
	wg.Go(func() { v.classify(false) })
	wg.Go(v.applicationStarted)
	wg.Wait()
	if shows.Load() != 1 {
		t.Fatalf("normal launch opened %d times", shows.Load())
	}
}
