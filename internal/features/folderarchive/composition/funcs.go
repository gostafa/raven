// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	boltstore "raven/internal/shared/boltstore/adapters/bbolt"
)

func archiveLibrary(store *boltstore.Store) *boltstore.Store { return store }
