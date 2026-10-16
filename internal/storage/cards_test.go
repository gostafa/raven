package storage

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"
	bolt "go.etcd.io/bbolt"
)

func TestCardsPersistWithReviewSchedule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raven.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	folder, err := store.CreateFolder(CreateFolderInput{Title: "Spanish"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := store.CreateCard(CreateCardInput{FolderID: folder.ID, Items: textItems("  See you tomorrow? ", " Hasta mañana. ")})
	if err != nil {
		t.Fatal(err)
	}
	if card.Items[0].Text != "See you tomorrow?" || card.Items[1].Text != "Hasta mañana." || card.LastReviewed != 0 {
		t.Fatalf("unexpected new card: %+v", card)
	}
	initial := card.Items[0]
	expected, err := fsrs.NewFSRS(fsrs.DefaultParam()).Next(fsrs.NewCard(initial.Schedule.LastReview), initial.Schedule.LastReview, fsrs.Again)
	if err != nil || !reflect.DeepEqual(initial.Schedule, expected.Card) || !initial.Reviews[0].Automatic {
		t.Fatal("initialization differs from FSRS Again")
	}
	sibling := card.Items[1]
	now := initial.Schedule.Due
	for _, rating := range []int{1, 2, 3, 4} {
		before := card.Items[0].Schedule
		expected, err := fsrs.NewFSRS(fsrs.DefaultParam()).Next(before, now, fsrs.Rating(rating))
		if err != nil {
			t.Fatal(err)
		}
		card, err = store.reviewItemAt(folder.ID, card.ID, initial.ID, rating, now)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(card.Items[0].Schedule, expected.Card) || !reflect.DeepEqual(card.Items[1], sibling) || card.LastReviewed != now.UnixMilli() {
			t.Fatal("rating differs from FSRS or changed sibling")
		}
		now = expected.Card.Due
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := store.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []Card{card}) {
		t.Fatalf("cards after reopen: %+v, %v", cards, err)
	}

}

func TestCardsValidateAndStayInTheirFolder(t *testing.T) {
	store := openTestStore(t)
	first, _ := store.CreateFolder(CreateFolderInput{Title: "First"})
	second, _ := store.CreateFolder(CreateFolderInput{Title: "Second"})
	for _, input := range []CreateCardInput{
		{FolderID: first.ID, Items: textItems(" ", "answer")},
		{FolderID: first.ID, Items: textItems("question", " \n")},
		{FolderID: "missing", Items: textItems("question", "answer")},
	} {
		if _, err := store.CreateCard(input); err == nil {
			t.Fatalf("accepted invalid card: %+v", input)
		}
	}
	cards, err := store.ListCards()
	if err != nil || cards == nil || len(cards) != 0 {
		t.Fatalf("failed save wrote cards: %+v, %v", cards, err)
	}
	card, err := store.CreateCard(CreateCardInput{FolderID: first.ID, Items: textItems("Question", "Answer")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewItem(second.ID, card.ID, card.Items[0].ID, 3); err == nil {
		t.Fatal("reviewed a card through the wrong folder")
	}
	if _, err := store.ReviewItem(first.ID, "missing", card.Items[0].ID, 3); err == nil {
		t.Fatal("reviewed a missing card")
	}
	cards, err = store.ListCards()
	if err != nil || !reflect.DeepEqual(cards, []Card{card}) {
		t.Fatalf("failed review changed cards: %+v, %v", cards, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListCards(); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("closed list: %v", err)
	}
	if _, err := store.CreateCard(CreateCardInput{FolderID: first.ID, Items: textItems("Question", "Answer")}); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("closed create: %v", err)
	}
	if _, err := store.ReviewItem(first.ID, card.ID, card.Items[0].ID, 3); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("closed review: %v", err)
	}
}

func textItems(texts ...string) []ItemDraft {
	items := make([]ItemDraft, 0, len(texts))
	for _, text := range texts {
		items = append(items, ItemDraft{Kind: "text", Text: text})
	}
	return items
}
