package domain

import (
	"testing"

	contentapi "raven/api/content"
)

func TestInvalidNormalizedPreparationStateIsRejected(t *testing.T) {
	if _, paths, err := finishPreparation(
		t.Context(),
		&mediaPreparation{rules: "invalid"},
	); err == nil ||
		paths != nil {
		t.Fatal("malformed normalized policy was accepted", paths, err)
	}
	if indexedItems("invalid") != nil {
		t.Fatal("malformed ownership index was accepted")
	}
	preparation := &mediaPreparation{items: []contentapi.Item{{Schedule: make(chan struct{})}}}
	if _, path, err := importItem(preparation, 0); err == nil || path != "" {
		t.Fatal("unsupported content reached media import", path, err)
	}
}
