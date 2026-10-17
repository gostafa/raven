package composition_test

import (
	"testing"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	configyaml "raven/internal/features/configuration/adapters/yaml"
	"raven/internal/shared/boltstore/ports"
	"raven/internal/shared/scheduling/composition"
)

func TestModuleProvidesConfiguredScheduler(t *testing.T) {
	settings, err := (configyaml.Loader{}).Load()
	if err != nil {
		t.Fatal(err)
	}
	var scheduler ports.Scheduler
	graph := fxtest.New(t, composition.Module, fx.Supply(settings), fx.Populate(&scheduler))
	graph.RequireStart()
	t.Cleanup(func() { graph.RequireStop() })
	now := time.Now()
	schedule, _, err := scheduler.Initialize(now)
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Validate(&schedule); err != nil {
		t.Fatal(err)
	}
	next, _, err := scheduler.Next(&schedule, now, settings.Library.MinimumRating)
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Validate(&next); err != nil {
		t.Fatal(err)
	}
}
