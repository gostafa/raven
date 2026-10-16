// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package library

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func ValidateItem(item ContentItem) error {
	err := validateItem(&item)
	if err != nil {
		return fmt.Errorf("validate content item: %w", err)
	}

	return nil
}

func validateItem(item *ContentItem) error {
	if item.ID == "" || item.Position < 0 {
		return errInvalidContentItem
	}

	if item.Kind == textKind {
		forwardErr := validateText(item)
		if forwardErr != nil {
			return fmt.Errorf("validate item: %w", forwardErr)
		}

		return nil
	}

	forwardErr2 := validateMedia(item)
	if forwardErr2 != nil {
		return fmt.Errorf("validate item: %w", forwardErr2)
	}

	return nil
}

func validateText(item *ContentItem) error {
	if strings.TrimSpace(item.Text) == "" || item.RelativePath != "" {
		return errInvalidTextItem
	}

	if item.MIMEType != "" || item.OriginalName != "" || item.ByteSize != 0 {
		return errInvalidTextItem
	}

	return nil
}

func validateMedia(item *ContentItem) error {
	if !isMediaKind(item.Kind) {
		return errInvalidContentKind
	}

	if item.Text != "" || item.OriginalName == "" || item.ByteSize <= 0 {
		return errInvalidMediaItem
	}

	forwardErr4 := validateMediaLocation(item)
	if forwardErr4 != nil {
		return fmt.Errorf("validate media: %w", forwardErr4)
	}

	return nil
}

func isMediaKind(kind string) bool { return kind == "audio" || kind == "image" || kind == "video" }

func validateMediaLocation(item *ContentItem) error {
	if !strings.HasPrefix(item.MIMEType, item.Kind+"/") ||
		!ValidMediaPath(item.RelativePath, item.ID) {
		return errInvalidMediaItem
	}

	return nil
}

func ValidMediaPath(path, id string) bool {
	if !itemIDPattern.MatchString(id) {
		return false
	}

	extension := filepath.Ext(path)

	return MediaType(extension) != "" && path == "media/"+id+extension
}

// MediaType returns the supported MIME type for a lowercase file extension.
// Active document formats, including SVG and HTML, are deliberately unsupported.
func MediaType(extension string) string {
	if mediaType := imageMediaType(extension); mediaType != "" {
		return mediaType
	}

	if mediaType := audioMediaType(extension); mediaType != "" {
		return mediaType
	}

	return videoMediaType(extension)
}

func imageMediaType(extension string) string {
	return map[string]string{
		".jpg": mimeJPEG, ".jpeg": mimeJPEG, ".png": "image/png",
		".gif": "image/gif", ".webp": "image/webp", ".avif": "image/avif",
	}[extension]
}

func audioMediaType(extension string) string {
	return map[string]string{
		".mp3": "audio/mpeg", ".wav": "audio/wav", ".m4a": "audio/mp4",
		".ogg": mimeOggAudio, ".oga": mimeOggAudio, ".flac": "audio/flac",
	}[extension]
}

func videoMediaType(extension string) string {
	return map[string]string{
		".mp4": mimeMP4Video, ".m4v": mimeMP4Video, ".webm": "video/webm",
		".ogv": "video/ogg", ".mov": "video/quicktime",
	}[extension]
}

func NormalizeFolder(input CreateFolderInput) (Folder, error) {
	folder := Folder{
		ID:          "",
		Title:       strings.TrimSpace(input.Title),
		Description: strings.TrimSpace(input.Description),
		Tags:        normalizeTags(input.Tags),
	}
	if folder.Title == "" {
		return Folder{}, errEnterAFolderTitle
	}

	return folder, nil
}

func normalizeTags(tags []string) []string {
	result := []string{}
	seen := map[string]bool{}

	for _, tag := range tags {
		normalized := strings.TrimSpace(tag)
		key := strings.ToLower(normalized)

		if normalized != "" && !seen[key] {
			result = append(result, normalized)
			seen[key] = true
		}
	}

	return result
}

func Hydrate(card Card) (Card, error) {
	err := orderItems(&card)
	if err != nil {
		return Card{}, fmt.Errorf("hydrate card: %w", err)
	}

	aggregateSchedule(&card)

	return card, nil
}

func orderItems(card *Card) error {
	sort.Slice(
		card.Items,
		func(first, second int) bool { return card.Items[first].Position < card.Items[second].Position },
	)

	for position := range card.Items {
		if card.Items[position].Position != position {
			return errInvalidContentOrder
		}
	}

	if len(card.Items) == 0 {
		return errAddSomethingYouWantToRemember
	}

	return nil
}

func aggregateSchedule(card *Card) {
	card.DueAt = 0
	card.LastReviewed = 0

	for index := range card.Items {
		updateDue(card, &card.Items[index])
		updateLastReview(card, card.Items[index].Reviews)
	}
}

func updateDue(card *Card, item *ContentItem) {
	due := item.Schedule.Due.UnixMilli()
	if card.DueAt == 0 || due < card.DueAt {
		card.DueAt = due
	}
}

func updateLastReview(card *Card, reviews []ItemReview) {
	for index := range reviews {
		review := &reviews[index]
		if !review.Automatic {
			card.LastReviewed = max(card.LastReviewed, review.Log.Review.UnixMilli())
		}
	}
}

func InitializeItem(item *ContentItem, now time.Time, scheduler Scheduler) error {
	schedule, log, err := scheduler.Initialize(now)
	if err != nil {
		return fmt.Errorf("initialize item schedule: %w", err)
	}

	item.Schedule = schedule
	item.Reviews = []ItemReview{{Log: log, Automatic: true}}
	item.LastNotifiedDue = 0

	return nil
}

func ApplyReview(item *ContentItem, now time.Time, rating int, scheduler Scheduler) error {
	if rating < minimumRating || rating > maximumRating {
		return errInvalidReviewRating
	}

	schedule, log, err := scheduler.Next(item.Schedule, now, rating)
	if err != nil {
		return fmt.Errorf("advance item schedule: %w", err)
	}

	item.Schedule = schedule
	item.Reviews = append(item.Reviews, ItemReview{Log: log, Automatic: false})

	return nil
}

func PrepareItems(
	drafts []ItemDraft,
	previous []ContentItem,
	importMedia func(ContentItem, string) (ContentItem, string, error),
	newID func() (string, error),
) ([]ContentItem, []string, error) {
	preparation := itemPreparation{
		previous:    indexItems(previous),
		seen:        make(map[string]bool),
		importMedia: importMedia,
		newID:       newID,
		imported:    []string{},
	}

	items, err := preparation.prepareAll(drafts)
	if err != nil {
		return nil, preparation.imported, fmt.Errorf("prepare card content: %w", err)
	}

	return items, preparation.imported, nil
}

func indexItems(items []ContentItem) map[string]ContentItem {
	result := make(map[string]ContentItem, len(items))
	for index := range items {
		result[items[index].ID] = items[index]
	}

	return result
}

func (preparation *itemPreparation) prepareAll(drafts []ItemDraft) ([]ContentItem, error) {
	items := make([]ContentItem, 0, len(drafts))
	for index := range drafts {
		item, err := preparation.prepare(&drafts[index], index)
		if err != nil {
			return nil, fmt.Errorf("prepare item: %w", err)
		}

		items = append(items, item)
	}

	if len(items) == 0 {
		return nil, errAddSomethingYouWantToRemember
	}

	return items, nil
}

func (preparation *itemPreparation) prepare(draft *ItemDraft, position int) (ContentItem, error) {
	item, err := preparation.resolve(draft)
	if err != nil {
		return ContentItem{}, fmt.Errorf("resolve draft: %w", err)
	}

	item.Position = position

	err = applyDraft(&item, draft)
	if err != nil {
		return ContentItem{}, fmt.Errorf("apply draft: %w", err)
	}

	preparation.seen[item.ID] = true

	return item, nil
}

func (preparation *itemPreparation) resolve(draft *ItemDraft) (ContentItem, error) {
	if draft.ID != "" {
		result2, forwardErr3 := preparation.retain(draft)
		if forwardErr3 != nil {
			return result2, fmt.Errorf("resolve: %w", forwardErr3)
		}

		return result2, nil
	}

	item, err := preparation.create(draft)
	if err != nil {
		return ContentItem{}, fmt.Errorf("create draft content: %w", err)
	}

	return item, nil
}

func (preparation *itemPreparation) retain(draft *ItemDraft) (ContentItem, error) {
	item, exists := preparation.previous[draft.ID]
	if !exists || preparation.seen[draft.ID] {
		return ContentItem{}, errInvalidExistingItemOrOwnership
	}

	if item.Kind != draft.Kind || draft.SourcePath != "" {
		return ContentItem{}, errInvalidExistingItemOrOwnership
	}

	return item, nil
}

func (preparation *itemPreparation) create(draft *ItemDraft) (ContentItem, error) {
	var item ContentItem

	id, err := preparation.newID()
	if err != nil {
		return item, fmt.Errorf("create item identifier: %w", err)
	}

	item.ID = id
	item.Kind = draft.Kind

	if draft.Kind == textKind {
		return item, nil
	}

	result3, forwardErr6 := preparation.importItem(&item, draft.SourcePath)
	if forwardErr6 != nil {
		return result3, fmt.Errorf("create: %w", forwardErr6)
	}

	return result3, nil
}

func (preparation *itemPreparation) importItem(
	item *ContentItem,
	source string,
) (ContentItem, error) {
	imported, path, err := preparation.importMedia(*item, source)
	if err != nil {
		return ContentItem{}, fmt.Errorf("import draft media: %w", err)
	}

	preparation.imported = append(preparation.imported, path)

	return imported, nil
}

func applyDraft(item *ContentItem, draft *ItemDraft) error {
	err := applyDraftContent(item, draft)
	if err != nil {
		return fmt.Errorf("apply content: %w", err)
	}

	err = validateItem(item)
	if err != nil {
		return fmt.Errorf("validate prepared content: %w", err)
	}

	return nil
}

func applyDraftContent(item *ContentItem, draft *ItemDraft) error {
	if draft.Kind == textKind {
		forwardErr5 := applyTextDraft(item, draft)
		if forwardErr5 != nil {
			return fmt.Errorf("apply draft content: %w", forwardErr5)
		}

		return nil
	}

	if draft.Text != "" {
		return errMediaCannotContainText
	}

	return nil
}

func applyTextDraft(item *ContentItem, draft *ItemDraft) error {
	if draft.SourcePath != "" {
		return errTextCannotReferenceAFile
	}

	item.Text = strings.TrimSpace(draft.Text)

	return nil
}
