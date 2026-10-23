package composition

import (
	"testing"

	boltstore "raven/internal/shared/boltstore/adapters/bbolt"
)

func TestArchiveProviders(t *testing.T) {
	store := &boltstore.Store{}
	if archiveLibrary(store) != store {
		t.Fatal("persistence port is not bound to the shared store")
	}
}
