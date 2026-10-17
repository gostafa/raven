package application_test

import (
	"errors"
	"testing"

	"raven/internal/features/configuration/adapters/yaml"
	"raven/internal/features/configuration/application"
)

type loader struct {
	snapshot *yaml.Settings
	err      error
}

func (source loader) Load() (*yaml.Settings, error) { return source.snapshot, source.err }

func TestLoadPreservesTheValidatedSnapshotAndRejectsFailures(t *testing.T) {
	snapshot, err := yaml.Load()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := application.Load(loader{snapshot: snapshot}.Load)
	if err != nil || loaded != snapshot {
		t.Fatal("configuration was not preserved", err)
	}
	cause := errors.New("invalid configuration")
	loaded, err = application.Load(loader{snapshot: snapshot, err: cause}.Load)
	if !errors.Is(err, cause) || loaded != nil {
		t.Fatal("load failure returned a snapshot or lost its cause", err)
	}
}
