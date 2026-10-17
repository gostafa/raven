package application_test

import (
	"errors"
	"testing"

	"raven/internal/features/configuration/adapters/yaml"
	"raven/internal/features/configuration/application"
	"raven/internal/features/configuration/ports"
)

type loader struct {
	snapshot ports.Snapshot
	err      error
}

func (source loader) Load() (ports.Snapshot, error) { return source.snapshot, source.err }

func TestLoadPreservesTheValidatedSnapshotAndRejectsFailures(t *testing.T) {
	snapshot, err := (yaml.Loader{}).Load()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := application.Load(loader{snapshot: snapshot})
	if err != nil || loaded.Settings != snapshot.Settings {
		t.Fatal("configuration was not preserved", err)
	}
	cause := errors.New("invalid configuration")
	loaded, err = application.Load(loader{snapshot: snapshot, err: cause})
	if !errors.Is(err, cause) || loaded.Settings != nil {
		t.Fatal("load failure returned a snapshot or lost its cause", err)
	}
}
