package content_test

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"

	cardapi "raven/api/cards"
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

func TestJSONContentPreservesLargeIntegerRecords(t *testing.T) {
	typedCard := cardapi.Card{}
	decodedCard, err := content.Decode[cardapi.Card](typedCard)
	if err != nil || !reflect.DeepEqual(decodedCard, typedCard) {
		t.Fatal("accepted typed card changed", err)
	}

	for _, values := range []struct {
		signed   int64
		unsigned uint64
	}{
		{signed: math.MaxInt64, unsigned: 9007199254740993},
		{signed: math.MinInt64, unsigned: math.MaxUint64},
	} {
		original := content.Item{
			LastNotifiedDue: values.signed,
			Schedule:        reviewapi.Schedule{Reps: values.unsigned, Lapses: values.unsigned},
			Reviews:         []content.Review{{Log: reviewapi.Log{ScheduledDays: values.unsigned}}},
		}
		encoded, err := json.Marshal(cardapi.Card{Items: []content.Item{original}})
		if err != nil {
			t.Fatal(err)
		}
		card, err := content.Decode[cardapi.Card](json.RawMessage(encoded))
		if err != nil {
			t.Fatal(err)
		}
		if err := cardapi.Normalize(&card); err != nil {
			t.Fatal(err)
		}
		decoded := content.Items(card.Items)[0]
		if !reflect.DeepEqual(decoded, original) {
			t.Fatalf("integer content changed: %#v != %#v", decoded, original)
		}
	}
}

func TestJSONDecoderRejectsTrailingDocumentsAndMalformedInput(t *testing.T) {
	for _, invalid := range []string{"", "{", `{} {}`, `{} true`, `{} trailing`} {
		var target any
		if err := content.DecodeJSON([]byte(invalid), &target); err == nil {
			t.Fatalf("accepted invalid JSON document: %q", invalid)
		}
	}
	var target map[string]any
	if err := content.DecodeJSON([]byte(`[]`), &target); err == nil {
		t.Fatal("accepted valid JSON with an incompatible target")
	}
	if err := content.DecodeJSON([]byte("{} \n\t"), &target); err != nil {
		t.Fatal("rejected trailing JSON whitespace", err)
	}
}

func TestContentHelpersDecodeJSONValues(t *testing.T) {
	const largeInteger = uint64(9007199254740993)
	var generic map[string]any
	if err := content.DecodeJSON([]byte(`{"count":9007199254740993}`), &generic); err != nil {
		t.Fatal("decoded generic JSON value", err)
	}
	if number, ok := generic["count"].(json.Number); !ok || number.String() != "9007199254740993" {
		t.Fatalf("generic JSON integer lost precision: %#v", generic["count"])
	}

	schedule := content.Schedule(map[string]any{"reps": largeInteger, "lapses": largeInteger})
	if schedule.Reps != largeInteger || schedule.Lapses != largeInteger {
		t.Fatalf("schedule integer changed: %#v", schedule)
	}
	log := content.Log(map[string]any{"scheduled_days": largeInteger, "rating": 4})
	if log.ScheduledDays != largeInteger || log.Rating != 4 {
		t.Fatalf("review log changed: %#v", log)
	}

	reviews := []content.Review{{Log: reviewapi.Log{Rating: 4}}}
	if got := content.Reviews(reviews); !reflect.DeepEqual(got, reviews) {
		t.Fatalf("validated reviews changed: %#v", got)
	}
	if content.Reviews("invalid") != nil {
		t.Fatal("accepted unvalidated review history")
	}
	if content.Items("invalid") != nil {
		t.Fatal("accepted unvalidated item list")
	}
}
