package cards_test

import (
	"encoding/json"
	"testing"

	"raven/api/cards"
	"raven/api/content"
)

func TestCardCodecsValidateContentAndPreserveMetadata(t *testing.T) {
	var card cards.Card
	if err := json.Unmarshal(
		[]byte(
			`{"id":"card","folder_id":"folder","created_at":1,"due_at":2,"last_reviewed":3,"items":[{"id":"item","kind":"text","text":"question"}]}`,
		),
		&card,
	); err != nil {
		t.Fatal(err)
	}
	if err := cards.Normalize(&card); err != nil {
		t.Fatal(err)
	}
	if card.ID != "card" || card.FolderID != "folder" || card.CreatedAt != 1 || card.DueAt != 2 ||
		card.LastReviewed != 3 ||
		len(content.Items(card.Items)) != 1 {
		t.Fatal("card metadata changed", card)
	}
	var create cards.CreateInput
	if err := json.Unmarshal(
		[]byte(`{"folder_id":"folder","items":[{"kind":"text","text":"question"}]}`),
		&create,
	); err != nil {
		t.Fatal(err)
	}
	var update cards.UpdateInput
	if err := json.Unmarshal(
		[]byte(
			`{"folder_id":"folder","card_id":"card","items":[{"id":"item","kind":"text","text":"edited"}]}`,
		),
		&update,
	); err != nil {
		t.Fatal(err)
	}
	if create.FolderID != "folder" || len(content.Drafts(create.Items)) != 1 ||
		update.CardID != "card" ||
		update.FolderID != "folder" ||
		content.Drafts(update.Items)[0].Text != "edited" {
		t.Fatal("card inputs changed")
	}
}

func TestCardCodecsRejectMalformedInputs(t *testing.T) {
	for _, target := range []any{&cards.Card{}, &cards.CreateInput{}, &cards.UpdateInput{}} {
		for _, data := range []string{`[]`, `{"folder_id":1}`, `{"items":"wrong"}`} {
			err := json.Unmarshal([]byte(data), target)
			if err == nil {
				err = normalize(target)
			}
			if err == nil {
				t.Fatalf("accepted %s into %T", data, target)
			}
		}
		if err := json.Unmarshal([]byte(`null`), target); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []any{nil, []content.Item{{Schedule: "wrong"}}} {
		card := cards.Card{Items: value}
		err := cards.Normalize(&card)
		if value == nil {
			if err != nil || card.Items != nil {
				t.Fatal("empty content changed", err)
			}
		} else if err == nil {
			t.Fatal("invalid nested content accepted")
		}
	}
}

func normalize(target any) error {
	switch value := target.(type) {
	case *cards.Card:
		return cards.Normalize(value)
	case *cards.CreateInput:
		return content.NormalizeDrafts(&value.Items)
	case *cards.UpdateInput:
		return content.NormalizeDrafts(&value.Items)
	}
	panic("unexpected test input")
}
