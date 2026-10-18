package composition

import (
	"errors"
	"testing"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"raven/internal/features/configuration/adapters/yaml"
	"raven/internal/features/configuration/domain"
)

type testLoader struct {
	calls   int
	failure error
}

func (loader *testLoader) Load() (*yaml.Settings, error) {
	loader.calls++
	if loader.failure != nil {
		return nil, loader.failure
	}
	return yaml.Load()
}

func TestGraphLoadsOnceAndSharesSnapshot(t *testing.T) {
	loader := &testLoader{}
	var first, second domain.Settings
	graph := fxtest.New(t, Module,
		fx.Replace(loader.Load),
		fx.Invoke(func(settings domain.Settings) { first = settings }),
		fx.Invoke(func(settings domain.Settings) { second = settings }),
	)
	graph.RequireStart().RequireStop()
	if loader.calls != 1 || first == nil || first != second {
		t.Fatal("configuration was not shared")
	}
}

func TestLoadPreservesFailureAndDefaultLoaderWorks(t *testing.T) {
	cause := errors.New("invalid embedded configuration")
	if settings, err := load(
		(&testLoader{failure: cause}).Load,
	); settings.Settings != nil ||
		!errors.Is(err, cause) {
		t.Fatal(settings, err)
	}
	if settings, err := load(newLoader()); settings.Settings == nil || err != nil {
		t.Fatal(settings, err)
	}
}
