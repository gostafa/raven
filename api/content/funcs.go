// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package content

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	reviewapi "raven/api/reviews"
)

// Decode validates a JSON-valued record against the requested schema.
func Decode[Value any](value any) (Value, error) {
	if decoded, ok := value.(Value); ok {
		return decoded, nil
	}

	var decoded Value

	data, err := json.Marshal(value)
	if err != nil {
		return decoded, fmt.Errorf("encode JSON value: %w", err)
	}

	err = DecodeJSON(data, &decoded)
	if err != nil {
		return decoded, fmt.Errorf("decode JSON value: %w", err)
	}

	return decoded, nil
}

// DecodeJSON preserves integer values in nested records and rejects trailing JSON.
func DecodeJSON(data []byte, target any) error {
	if !json.Valid(data) {
		return errors.Join(json.Unmarshal(data, target))
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	err := decoder.Decode(target)
	if err != nil {
		return fmt.Errorf("decode JSON record: %w", err)
	}

	return nil
}

// Items reads the ordered items of an already validated card record.
func Items(value any) []Item {
	if items, ok := value.([]Item); ok {
		return items
	}

	return nil
}

// Drafts reads JSON-valued drafts, rejecting values outside their schema.
func Drafts(value any) []Draft {
	drafts, err := Decode[[]Draft](value)
	if err != nil {
		return nil
	}

	return drafts
}

// NormalizeDrafts validates JSON-valued input content before a card mutation.
func NormalizeDrafts(value *any) error {
	drafts, err := Decode[[]Draft](*value)
	if err != nil {
		return fmt.Errorf("validate content drafts: %w", err)
	}

	*value = drafts

	return nil
}

// Reviews reads the history of an already validated content record.
func Reviews(value any) []Review {
	if reviews, ok := value.([]Review); ok {
		return reviews
	}

	return nil
}

// Schedule reads the scheduling state of an already validated content record.
func Schedule(value any) reviewapi.Schedule {
	schedule, err := Decode[reviewapi.Schedule](value)
	if err != nil {
		var zero reviewapi.Schedule

		return zero
	}

	return schedule
}

// Log reads the scheduling decision of an already validated history record.
func Log(value any) reviewapi.Log {
	log, err := Decode[reviewapi.Log](value)
	if err != nil {
		var zero reviewapi.Log

		return zero
	}

	return log
}

// NormalizeItem validates JSON-valued scheduling and history fields before use.
func NormalizeItem(item *Item) error {
	schedule, err := Decode[reviewapi.Schedule](item.Schedule)
	if err != nil {
		return fmt.Errorf("validate content schedule: %w", err)
	}

	item.Schedule = schedule

	err = normalizeHistory(item)
	if err != nil {
		return fmt.Errorf("validate content history: %w", err)
	}

	return nil
}

func normalizeHistory(item *Item) error {
	reviews, err := Decode[[]Review](item.Reviews)
	if err != nil {
		return fmt.Errorf("decode content history: %w", err)
	}

	item.Reviews = reviews

	for index := range reviews {
		err = NormalizeReview(&reviews[index])
		if err != nil {
			return fmt.Errorf("validate history entry: %w", err)
		}
	}

	return nil
}

// NormalizeReview validates a JSON-valued scheduling decision before use.
func NormalizeReview(review *Review) error {
	log, err := Decode[reviewapi.Log](review.Log)
	if err != nil {
		return fmt.Errorf("validate review log: %w", err)
	}

	review.Log = log

	return nil
}
