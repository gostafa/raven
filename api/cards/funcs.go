// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package cards

import (
	"fmt"

	contentapi "raven/api/content"
)

// Normalize validates JSON-valued content before exposing a card to workflows.
func Normalize(card *Card) error {
	items, err := contentapi.Decode[[]contentapi.Item](card.Items)
	if err != nil {
		return fmt.Errorf("decode card content: %w", err)
	}

	setContent(card, items)

	for index := range items {
		err = contentapi.NormalizeItem(&items[index])
		if err != nil {
			return fmt.Errorf("validate card content: %w", err)
		}
	}

	return nil
}

func setContent(card *Card, items []contentapi.Item) {
	card.Items = nil

	if len(items) == zero {
		return
	}

	card.Items = items
}
