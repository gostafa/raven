// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package library

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func ValidateItem(item ContentItem) error {
	if item.ID == "" || item.Position < 0 {
		return errors.New("invalid content item")
	}

	if item.Kind == "text" {
		if strings.TrimSpace(item.Text) == "" || item.RelativePath != "" || item.MIMEType != "" ||
			item.OriginalName != "" ||
			item.ByteSize != 0 {

			return errors.New("invalid text item")
		}

		return nil
	}

	if item.Kind != "audio" && item.Kind != "image" && item.Kind != "video" {
		return errors.New("invalid content kind")
	}

	if item.Text != "" || item.OriginalName == "" || item.ByteSize <= 0 ||
		!strings.HasPrefix(item.MIMEType, item.Kind+"/") ||
		!ValidMediaPath(item.RelativePath, item.ID) {

		return errors.New("invalid media item")
	}

	return nil
}

func ValidMediaPath(path, id string) bool {
	if !itemIDPattern.MatchString(id) {
		return false
	}

	ext := filepath.Ext(path)
	_, supported := mediaTypes[ext]

	return supported && path == "media/"+id+ext
}

func NormalizeFolder(input CreateFolderInput) (Folder, error) {
	folder := Folder{
		Title:       strings.TrimSpace(input.Title),
		Description: strings.TrimSpace(input.Description),
		Tags:        []string{},
	}
	if folder.Title == "" {
		return Folder{}, errors.New("enter a folder title")
	}

	seen := map[string]bool{}

	for _, tag := range input.Tags {
		tag = strings.TrimSpace(tag)

		key := strings.ToLower(tag)

		if tag != "" && !seen[key] {
			folder.Tags = append(folder.Tags, tag)
			seen[key] = true
		}
	}

	return folder, nil
}

func Hydrate(card Card) (Card, error) {
	sort.Slice(
		card.Items,
		func(i, j int) bool { return card.Items[i].Position < card.Items[j].Position },
	)

	for position, item := range card.Items {
		if item.Position != position {
			return Card{}, errors.New("invalid content order")
		}
	}

	if len(card.Items) == 0 {
		return Card{}, errors.New("add something you want to remember")
	}

	card.DueAt = 0
	card.LastReviewed = 0

	for _, item := range card.Items {
		due := item.Schedule.Due.UnixMilli()
		if card.DueAt == 0 || due < card.DueAt {
			card.DueAt = due
		}

		for _, review := range item.Reviews {
			if !review.Automatic && review.Log.Review.UnixMilli() > card.LastReviewed {
				card.LastReviewed = review.Log.Review.UnixMilli()
			}
		}
	}

	return card, nil
}

func InitializeItem(item *ContentItem, now time.Time, scheduler Scheduler) error {
	schedule, log, err := scheduler.Initialize(now)
	if err != nil {
		return err
	}

	item.Schedule = schedule
	item.Reviews = []ItemReview{{Log: log, Automatic: true}}
	item.LastNotifiedDue = 0

	return nil
}

func ApplyReview(item *ContentItem, now time.Time, rating int, scheduler Scheduler) error {
	if rating < 1 || rating > 4 {
		return errors.New("invalid review rating")
	}

	schedule, log, err := scheduler.Next(item.Schedule, now, rating)
	if err != nil {
		return err
	}

	item.Schedule = schedule
	item.Reviews = append(item.Reviews, ItemReview{Log: log})

	return nil
}

func PrepareItems(
	drafts []ItemDraft,
	previous []ContentItem,
	importMedia func(ContentItem, string) (ContentItem, string, error),
	newID func() (string, error),
) ([]ContentItem, []string, error) {
	items := make([]ContentItem, 0, len(drafts))
	imported := []string{}
	old := make(map[string]ContentItem)

	for _, item := range previous {
		old[item.ID] = item
	}

	seen := make(map[string]bool)

	for _, draft := range drafts {
		var item ContentItem

		if draft.ID != "" {
			var ok bool

			item, ok = old[draft.ID]

			if !ok || seen[draft.ID] || item.Kind != draft.Kind || draft.SourcePath != "" {
				return nil, imported, errors.New("invalid existing item or ownership")
			}
		} else {
			id, err := newID()
			if err != nil {
				return nil, imported, err
			}

			item = ContentItem{ID: id, Kind: draft.Kind}
			if draft.Kind != "text" {
				var path string

				item, path, err = importMedia(item, draft.SourcePath)
				if err != nil {
					return nil, imported, err
				}

				imported = append(imported, path)
			}
		}

		if draft.Kind == "text" {
			if draft.SourcePath != "" {
				return nil, imported, errors.New("text cannot reference a file")
			}

			item.Text = strings.TrimSpace(draft.Text)
		} else if draft.Text != "" {
			return nil, imported, errors.New("media cannot contain text")
		}

		item.Position = len(items)

		err := ValidateItem(item)
		if err != nil {
			return nil, imported, err
		}

		seen[item.ID] = true
		items = append(items, item)
	}

	if len(items) == 0 {
		return nil, imported, errors.New("add something you want to remember")
	}

	return items, imported, nil
}
