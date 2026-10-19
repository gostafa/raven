package domain

import (
	"testing"
	"time"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	reviewapi "raven/api/reviews"
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

func TestDraftCanSwitchExistingContentBetweenRecallAndHint(t *testing.T) {
	rules := &Rules{TextKind: "text"}
	item := contentapi.Item{ID: "item", Kind: "text", Text: "A clue"}
	for _, hint := range []bool{true, false} {
		draft := contentapi.Draft{Kind: "text", Text: item.Text, Hint: hint}
		if err := applyDraft(rules, &item, &draft); err != nil {
			t.Fatal(err)
		}
		if item.Hint != hint || item.Text != "A clue" {
			t.Fatalf("role change lost content: %+v", item)
		}
	}
}

func TestHintsDoNotSetCardDueTime(t *testing.T) {
	due := time.Unix(100, 0)
	card := cardapi.Card{Items: []contentapi.Item{
		{Hint: true, Schedule: reviewapi.Schedule{Due: time.Unix(1, 0)}},
		{Schedule: reviewapi.Schedule{Due: due}},
	}}
	aggregateSchedule(&card)
	if card.DueAt != due.UnixMilli() {
		t.Fatalf("hint affected due time: %d", card.DueAt)
	}
	card.Items = []contentapi.Item{{Hint: true}}
	aggregateSchedule(&card)
	if card.DueAt != 0 {
		t.Fatalf("hint-only card is due: %d", card.DueAt)
	}
}
