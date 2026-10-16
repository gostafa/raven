package application

import (
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

type fakeAutostart struct {
	enabled                  bool
	enables, disables, reads int
	options                  Options
	operationErr, statusErr  error
	ignoreEnable             bool
}

func (f *fakeAutostart) EnableWithOptions(options *Options) error {
	f.enables++
	f.options = *options
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
			c := &Controller{
				backend: backend,
				debug:   tc.debug,
				publish: func(b, a bool) { checked, available = b, a },
			}
			if err := c.Initialize(settings); err != nil {
				t.Fatal(err)
			}
			if checked != tc.wantChecked || available != tc.wantAvailable ||
				backend.enables != tc.wantEnables {
				t.Fatalf("checked=%v available=%v enables=%d", checked, available, backend.enables)
			}
			if tc.debug {
				_ = c.Toggle()
				_ = c.Refresh()
				if settings.calls != 0 || backend.reads != 0 || backend.enables != 0 ||
					backend.disables != 0 {
					t.Fatal("development touched OS settings")
				}
			}
			if tc.wantEnables > 0 &&
				!reflect.DeepEqual(
					backend.options,
					Options{
						Identifier: ApplicationID,
						Arguments:  []string{autostartArgument},
					},
				) {
				t.Fatalf("registration: %+v", backend.options)
			}
		})
	}
}

func TestAutostartTogglesAndExternalChangesSurviveRestart(t *testing.T) {
	backend, settings := &fakeAutostart{}, &fakeAutostartSettings{}
	checked := false
	newController := func() *Controller {
		return &Controller{backend: backend, publish: func(b, _ bool) { checked = b }}
	}
	c := newController()
	if err := c.Initialize(settings); err != nil {
		t.Fatal(err)
	}
	if err := c.Toggle(); err != nil || checked || backend.enabled {
		t.Fatalf("disable failed: %v", err)
	}
	c = newController()
	if err := c.Initialize(settings); err != nil || checked || backend.enables != 1 {
		t.Fatalf("restart undid opt-out: %v", err)
	}
	if err := c.Toggle(); err != nil || !checked {
		t.Fatalf("re-enable failed: %v", err)
	}
	backend.enabled = false
	if err := c.Refresh(); err != nil || checked || backend.enables != 2 {
		t.Fatalf("Refresh undid OS disable: %v", err)
	}
	backend.enabled = true // Checkbox is now stale.
	if err := c.Toggle(); err != nil || checked || backend.enabled {
		t.Fatalf("Toggle used stale state: %v", err)
	}
}

func TestAutostartFailuresAndExplicitRetry(t *testing.T) {
	failure := errors.New("permission denied")
	backend, settings := &fakeAutostart{operationErr: failure}, &fakeAutostartSettings{}
	c := &Controller{backend: backend}
	if err := c.Initialize(settings); !errors.Is(err, failure) || !settings.claimed || c.enabled {
		t.Fatalf("initial failure: %v", err)
	}
	backend.operationErr = nil
	c = &Controller{backend: backend}
	if err := c.Initialize(settings); err != nil || backend.enables != 1 {
		t.Fatalf("default retried automatically: %v", err)
	}
	if err := c.Toggle(); err != nil || !backend.enabled {
		t.Fatalf("explicit retry: %v", err)
	}
	backend.operationErr = failure
	if err := c.Toggle(); !errors.Is(err, failure) || !c.enabled {
		t.Fatalf("failed disable lost actual state: %v", err)
	}
	backend.statusErr = failure
	before := backend.disables
	if err := c.Toggle(); !errors.Is(err, failure) || backend.disables != before {
		t.Fatalf("mutated after status error: %v", err)
	}
}

func TestAutostartRequiresDurableMarkerAndEffectiveRegistration(t *testing.T) {
	failure := errors.New("database closed")
	backend := &fakeAutostart{}
	c := &Controller{backend: backend}
	if err := c.Initialize(
		&fakeAutostartSettings{err: failure},
	); !errors.Is(err, failure) || c.ready ||
		backend.reads != 0 {
		t.Fatalf("marker failure: %v", err)
	}
	_ = c.Toggle()
	if backend.enables != 0 {
		t.Fatal("Toggle bypassed failed marker")
	}
	c = &Controller{backend: &fakeAutostart{ignoreEnable: true}}
	if err := c.Initialize(&fakeAutostartSettings{}); err == nil || c.enabled {
		t.Fatal("reported success while OS approval still required")
	}
}

func TestAutostartSerializesMenuActions(t *testing.T) {
	backend := &fakeAutostart{}
	c := &Controller{backend: backend}
	if err := c.Initialize(&fakeAutostartSettings{}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if err := c.Toggle(); err != nil {
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
			v := &Visibility{show: func() { shows++ }}
			if nativeFirst {
				v.Classify(automatic)
			} else {
				v.ApplicationStarted()
			}
			if shows != 0 {
				t.Fatal("main opened before startup completed")
			}
			if nativeFirst {
				v.ApplicationStarted()
			} else {
				v.Classify(automatic)
			}
			want := 1
			if automatic {
				want = 0
			}
			v.ApplicationStarted()
			if shows != want {
				t.Fatalf("automatic=%v nativeFirst=%v shows=%d", automatic, nativeFirst, shows)
			}
		}
	}
}

func TestSecondInstanceVisibility(t *testing.T) {
	for _, early := range []bool{false, true} {
		shows := 0
		v := &Visibility{show: func() { shows++ }}
		if early {
			v.SecondInstance([]string{"raven"})
		}
		v.Classify(true)
		v.ApplicationStarted()
		want := 0
		if early {
			want = 1
		}
		v.SecondInstance([]string{"raven", autostartArgument})
		if shows != want {
			t.Fatal("automatic second launch opened main")
		}
		v.SecondInstance([]string{"raven"})
		if shows != want+1 {
			t.Fatal("normal second launch did not open main")
		}
	}
	for _, arg := range []string{"", "--autostart=false", "file--autostart"} {
		if IsAutostartLaunch([]string{arg}) {
			t.Fatalf("misclassified argument %q", arg)
		}
	}
}

func TestLaunchVisibilityConcurrentStartup(t *testing.T) {
	var shows atomic.Int32
	v := &Visibility{show: func() { shows.Add(1) }}
	var wg sync.WaitGroup
	wg.Go(func() { v.Classify(false) })
	wg.Go(v.ApplicationStarted)
	wg.Wait()
	if shows.Load() != 1 {
		t.Fatalf("normal launch opened %d times", shows.Load())
	}
}

func TestConstructedControllerReportsStatusFailuresAndUsesNewPublisher(t *testing.T) {
	failure := errors.New("status unavailable")
	backend := &fakeAutostart{statusErr: failure}
	controller := New(backend, false, nil)
	var published [][2]bool
	controller.SetPublisher(func(checked, available bool) {
		published = append(published, [2]bool{checked, available})
	})
	if err := controller.Initialize(&fakeAutostartSettings{}); !errors.Is(err, failure) {
		t.Fatalf("expected initialization read failure, got %v", err)
	}
	if err := controller.Refresh(); !errors.Is(err, failure) {
		t.Fatalf("expected refresh failure, got %v", err)
	}
	if backend.enables != 0 || len(published) != 2 {
		t.Fatalf("unexpected registration or publications: %+v, %v", backend, published)
	}
	backend.statusErr = nil
	backend.enabled = true
	if err := controller.Refresh(); err != nil {
		t.Fatal(err)
	}
	if published[len(published)-1] != [2]bool{true, true} {
		t.Fatal("new publisher did not receive the observed state")
	}
	shows := 0
	visibility := NewVisibility(func() { shows++ })
	visibility.Classify(false)
	visibility.ApplicationStarted()
	if shows != 1 {
		t.Fatal("constructed visibility did not open the first manual launch")
	}
}
