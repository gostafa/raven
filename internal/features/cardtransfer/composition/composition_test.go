package composition

import (
	"testing"

	boltstore "raven/internal/shared/boltstore/adapters/bbolt"
)

func TestTransferProvider(t *testing.T) {
	store := &boltstore.Store{}
	if transferLibrary(store) != store {
		t.Fatal("transfer library does not share persistence")
	}
}
