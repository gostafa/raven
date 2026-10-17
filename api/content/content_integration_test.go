package content_test

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"

	"raven/api/content"
	reviewapi "raven/api/reviews"
)

func TestValidatedContentRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	item := content.Item{
		ID:           "item",
		FolderID:     "folder",
		CardID:       "card",
		Kind:         "image",
		Text:         "caption",
		OriginalName: "test.png",
		MIMEType:     "image/png",
		RelativePath: "media/item.png",
		Reviews: []content.Review{
			{Log: reviewapi.Log{Due: now, Review: now, Rating: 3}, Automatic: true},
		},
		Schedule:        reviewapi.Schedule{Due: now, LastReview: now, Reps: 2},
		Position:        2,
		LastNotifiedDue: now.UnixMilli(),
		ByteSize:        12,
	}
	encoded, err := json.Marshal(&item)
	if err != nil {
		t.Fatal(err)
	}
	var decoded content.Item
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := content.NormalizeItem(&decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, item) {
		t.Fatalf("content changed: %#v != %#v", decoded, item)
	}
	if content.Schedule(decoded.Schedule).Reps != 2 ||
		content.Log(content.Reviews(decoded.Reviews)[0].Log).Rating != 3 {
		t.Fatal("nested values were not validated")
	}
	items := []content.Item{decoded}
	drafts := []content.Draft{{Kind: "text", Text: "question"}}
	if !reflect.DeepEqual(content.Items(items), items) ||
		!reflect.DeepEqual(content.Drafts(drafts), drafts) {
		t.Fatal("validated sequences changed")
	}
	if content.Items(nil) != nil || content.Drafts(nil) != nil || content.Drafts("wrong") != nil ||
		content.Reviews(nil) != nil ||
		content.Schedule(nil) != (reviewapi.Schedule{}) ||
		content.Log(nil) != (reviewapi.Log{}) {
		t.Fatal("zero values changed")
	}
}

func TestContentCodecsRejectMalformedNestedValues(t *testing.T) {
	for _, data := range []string{
		`[]`, `{"position":"wrong"}`, `{"schedule":"wrong"}`, `{"schedule":{"due":"wrong"}}`,
		`{"reviews":"wrong"}`, `{"reviews":[{"automatic":"wrong"}]}`, `{"reviews":[{"log":"wrong"}]}`,
	} {
		var item content.Item
		err := json.Unmarshal([]byte(data), &item)
		if err == nil {
			err = content.NormalizeItem(&item)
		}
		if err == nil {
			t.Fatalf("accepted malformed content %s", data)
		}
	}
	for _, data := range []string{`[]`, `{"automatic":"wrong"}`, `{"log":"wrong"}`} {
		var review content.Review
		err := json.Unmarshal([]byte(data), &review)
		if err == nil {
			err = content.NormalizeReview(&review)
		}
		if err == nil {
			t.Fatalf("accepted malformed review %s", data)
		}
	}
	var item content.Item
	if err := json.Unmarshal([]byte(`{"unknown":true}`), &item); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{make(chan int), "wrong"} {
		if _, err := content.Decode[reviewapi.Schedule](value); err == nil {
			t.Fatalf("accepted invalid schedule %#v", value)
		}
	}
}

func TestContentEncodingRejectsInvalidValues(t *testing.T) {
	for _, item := range []content.Item{
		{Schedule: make(chan int)},
		{Reviews: make(chan int)},
		{Schedule: reviewapi.Schedule{Stability: math.NaN()}},
	} {
		if _, err := json.Marshal(&item); err == nil {
			t.Fatal("encoded invalid content")
		}
	}
	for _, review := range []content.Review{
		{Log: make(chan int)}, {Log: reviewapi.Log{Stability: math.NaN()}},
	} {
		if _, err := json.Marshal(&review); err == nil {
			t.Fatal("encoded invalid review")
		}
	}
	for _, value := range []any{&content.Item{}, &content.Review{}} {
		switch record := value.(type) {
		case *content.Item:
			if err := content.NormalizeItem(record); err != nil {
				t.Fatal(err)
			}
		case *content.Review:
			if err := content.NormalizeReview(record); err != nil {
				t.Fatal(err)
			}
		}
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"schedule", "log"} {
			if encoded, present := fields[key]; present && string(encoded) == "null" {
				t.Fatalf("zero %s changed to null", key)
			}
		}
	}
}

func TestDraftNormalization(t *testing.T) {
	if content.Schedule("wrong") != (reviewapi.Schedule{}) ||
		content.Log("wrong") != (reviewapi.Log{}) {
		t.Fatal("invalid scheduling values accepted")
	}
	var value any = []map[string]any{{"kind": "text", "text": "question"}}
	if err := content.NormalizeDrafts(&value); err != nil || len(content.Drafts(value)) != 1 {
		t.Fatal("valid draft rejected", err)
	}
	value = "wrong"
	if err := content.NormalizeDrafts(&value); err == nil {
		t.Fatal("invalid drafts accepted")
	}
}
