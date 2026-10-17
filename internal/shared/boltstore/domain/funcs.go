// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package domain

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	reviewapi "raven/api/reviews"
	"raven/internal/shared/boltstore/ports"
)

// ValidateItem checks structural and kind-specific content invariants.
func (rules *Rules) ValidateItem(item *contentapi.Item) error {
	if item.ID == "" || item.Position < zero {
		return errInvalidContentItem
	}

	validate := rules.validateMedia

	if item.Kind == rules.TextKind {
		validate = validateText
	}

	err := validate(item)
	if err != nil {
		return fmt.Errorf("validate content item: %w", err)
	}

	return nil
}

func validateText(item *contentapi.Item) error {
	if strings.TrimSpace(item.Text) == "" || hasMediaMetadata(item) {
		return errInvalidTextItem
	}

	return nil
}

func hasMediaMetadata(item *contentapi.Item) bool {
	return item.RelativePath != "" || item.MIMEType != "" || item.OriginalName != "" ||
		item.ByteSize != zero
}

func (rules *Rules) validateMedia(item *contentapi.Item) error {
	if !isMediaKind(item.Kind) {
		return errInvalidContentKind
	}

	if !validMediaMetadata(item) {
		return errInvalidMediaItem
	}

	err := rules.validateMediaLocation(item)
	if err != nil {
		return fmt.Errorf("validate media: %w", err)
	}

	return nil
}

func validMediaMetadata(item *contentapi.Item) bool {
	return item.Text == "" && item.OriginalName != "" && item.ByteSize > zero
}

func isMediaKind(kind string) bool { return kind == "audio" || kind == "image" || kind == "video" }

func (rules *Rules) validateMediaLocation(item *contentapi.Item) error {
	if !rules.mediaLocationMatches(item) {
		return errInvalidMediaItem
	}

	return nil
}

func (rules *Rules) mediaLocationMatches(item *contentapi.Item) bool {
	return strings.HasPrefix(item.MIMEType, item.Kind+"/") &&
		rules.ValidMediaPath(item.RelativePath, item.ID)
}

// ValidMediaPath checks that a supported media path belongs to the item identifier.
func (rules *Rules) ValidMediaPath(path, id string) bool {
	if !itemIDPattern.MatchString(id) {
		return false
	}

	extension := filepath.Ext(path)

	return rules.MediaType(extension) != "" && path == "media/"+id+extension
}

// MediaType returns the supported MIME type for a lowercase file extension.
// Active document formats, including SVG and HTML, are deliberately unsupported.
func (rules *Rules) MediaType(extension string) string {
	if mediaType := rules.imageMediaType(extension); mediaType != "" {
		return mediaType
	}

	if mediaType := rules.audioMediaType(extension); mediaType != "" {
		return mediaType
	}

	return rules.videoMediaType(extension)
}

func (rules *Rules) imageMediaType(extension string) string {
	return map[string]string{
		".jpg": rules.MIMEJPEG, ".jpeg": rules.MIMEJPEG, ".png": "image/png",
		".gif": "image/gif", ".webp": "image/webp", ".avif": "image/avif",
	}[extension]
}

func (rules *Rules) audioMediaType(extension string) string {
	return map[string]string{
		".mp3": "audio/mpeg", ".wav": "audio/wav", ".m4a": "audio/mp4",
		".ogg": rules.MIMEOggAudio, ".oga": rules.MIMEOggAudio, ".flac": "audio/flac",
	}[extension]
}

func (rules *Rules) videoMediaType(extension string) string {
	return map[string]string{
		".mp4": rules.MIMEMP4Video, ".m4v": rules.MIMEMP4Video, ".webm": "video/webm",
		".ogv": "video/ogg", ".mov": "video/quicktime",
	}[extension]
}

// NormalizeFolder trims metadata and removes duplicate or blank tags.
func NormalizeFolder(input *folderapi.CreateInput) (folderapi.Folder, error) {
	folder := folderapi.Folder{
		ID:          "",
		Title:       strings.TrimSpace(input.Title),
		Description: strings.TrimSpace(input.Description),
		Tags:        normalizeTags(input.Tags),
	}
	if folder.Title == "" {
		return folderapi.Folder{}, errEnterAFolderTitle
	}

	return folder, nil
}

func normalizeTags(tags []string) []string {
	result := []string{}
	seen := map[string]bool{}

	for tagIndex := range tags {
		tag := &tags[tagIndex]

		normalized := strings.TrimSpace((*tag))
		key := strings.ToLower(normalized)

		if normalized != "" && !seen[key] {
			result = append(result, normalized)
			seen[key] = true
		}
	}

	return result
}

// Hydrate orders card items and computes aggregate scheduling timestamps.
func Hydrate(input *cardapi.Card) (cardapi.Card, error) {
	card := *input

	err := orderItems(&card)
	if err != nil {
		return cardapi.Card{}, fmt.Errorf("hydrate card: %w", err)
	}

	aggregateSchedule(&card)

	return card, nil
}

func orderItems(card *cardapi.Card) error {
	slices.SortFunc(
		card.Items,
		func(first, second contentapi.Item) int { return cmp.Compare(first.Position, second.Position) },
	)

	for position := range card.Items {
		if card.Items[position].Position != position {
			return errInvalidContentOrder
		}
	}

	if len(card.Items) == zero {
		return errAddSomethingYouWantToRemember
	}

	return nil
}

func aggregateSchedule(card *cardapi.Card) {
	card.DueAt = zero
	card.LastReviewed = zero

	for index := range card.Items {
		updateDue(card, &card.Items[index])
		updateLastReview(card, card.Items[index].Reviews)
	}
}

func updateDue(card *cardapi.Card, item *contentapi.Item) {
	dueMilli := item.Schedule.Due.UnixMilli()
	if card.DueAt == zero || dueMilli < card.DueAt {
		card.DueAt = dueMilli
	}
}

func updateLastReview(card *cardapi.Card, reviews []contentapi.Review) {
	for index := range reviews {
		review := &reviews[index]
		if !review.Automatic {
			card.LastReviewed = max(card.LastReviewed, review.Log.Review.UnixMilli())
		}
	}
}

// InitializeItem creates the initial schedule and automatic review record.
func InitializeItem(item *contentapi.Item, now time.Time, scheduler ports.Scheduler) error {
	schedule, log, err := scheduler.Initialize(now)
	if err != nil {
		return fmt.Errorf("initialize item schedule: %w", err)
	}

	item.Schedule = schedule
	item.Reviews = []contentapi.Review{{Log: log, Automatic: true}}
	item.LastNotifiedDue = zero

	return nil
}

// ApplyReview advances a schedule after a validated user rating.
func (rules *Rules) ApplyReview(
	item *contentapi.Item,
	attempt *reviewapi.Attempt,
	scheduler ports.Scheduler,
) error {
	if attempt.Rating < rules.MinimumRating || attempt.Rating > rules.MaximumRating {
		return errInvalidReviewRating
	}

	current := item.Schedule

	schedule, log, err := scheduler.Next(&current, attempt.At, attempt.Rating)
	if err != nil {
		return fmt.Errorf("advance item schedule: %w", err)
	}

	item.Schedule = schedule
	item.Reviews = append(item.Reviews, contentapi.Review{Log: log, Automatic: false})

	return nil
}

// PrepareItems validates drafts and returns newly imported files for rollback.
func (rules *Rules) PrepareItems(
	ctx context.Context,
	input *PreparationInput,
	dependencies *PreparationDependencies,
) ([]contentapi.Item, []string, error) {
	items, err := resolveDrafts(input, dependencies)
	if err != nil {
		return nil, nil, fmt.Errorf("prepare card content: %w", err)
	}

	media := rules.newMediaPreparation(items, input, dependencies)

	result, imported, err := rules.finishPreparation(ctx, media)
	if err != nil {
		return nil, imported, fmt.Errorf("finish card content: %w", err)
	}

	return result, imported, nil
}

func resolveDrafts(
	input *PreparationInput,
	dependencies *PreparationDependencies,
) ([]contentapi.Item, error) {
	preparation := itemPreparation{
		previous: indexItems(
			input.Previous,
		),
		seen:  make(map[string]bool),
		newID: dependencies.NewID,
	}

	items, err := preparation.prepareAll(input.Drafts)
	if err != nil {
		return nil, fmt.Errorf("resolve content drafts: %w", err)
	}

	return items, nil
}

func (rules *Rules) newMediaPreparation(
	items []contentapi.Item,
	input *PreparationInput,
	dependencies *PreparationDependencies,
) *mediaPreparation {
	return &mediaPreparation{
		items: items, drafts: input.Drafts, dependencies: dependencies,
		textKind: rules.TextKind, paths: []string{},
	}
}

func (rules *Rules) finishPreparation(
	ctx context.Context,
	media *mediaPreparation,
) ([]contentapi.Item, []string, error) {
	imported, err := media.importAll(ctx)
	if err != nil {
		return nil, imported, fmt.Errorf("import card content: %w", err)
	}

	err = rules.applyDrafts(media.items, media.drafts)
	if err != nil {
		return nil, imported, fmt.Errorf("apply drafts: %w", err)
	}

	return media.items, imported, nil
}

func (rules *Rules) applyDrafts(items []contentapi.Item, drafts []contentapi.Draft) error {
	for index := range items {
		err := rules.applyDraft(&items[index], &drafts[index])
		if err != nil {
			return fmt.Errorf("apply draft: %w", err)
		}
	}

	return nil
}

func (preparation *mediaPreparation) importAll(ctx context.Context) ([]string, error) {
	jobs := preparation.jobs()
	err := preparation.dependencies.Batch.Run(ctx, jobs)
	imported := preparation.importedPaths()

	if err != nil {
		return imported, fmt.Errorf("run media imports: %w", err)
	}

	return imported, nil
}

func (preparation *mediaPreparation) jobs() []func(context.Context) error {
	jobs := make([]func(context.Context) error, zero, len(preparation.items))
	for index := range preparation.items {
		preparation.paths = append(preparation.paths, "")
		if preparation.needsImport(index) {
			jobs = append(jobs, preparation.importJob(index))
		}
	}

	return jobs
}

func (preparation *mediaPreparation) needsImport(index int) bool {
	return preparation.drafts[index].ID == "" &&
		preparation.items[index].Kind != preparation.textKind
}

func (preparation *mediaPreparation) importJob(index int) func(context.Context) error {
	return func(context.Context) error {
		imported, path, err := preparation.dependencies.ImportMedia(
			&preparation.items[index],
			preparation.drafts[index].SourcePath,
		)

		preparation.paths[index] = path

		if err != nil {
			return fmt.Errorf("import draft media: %w", err)
		}

		preparation.items[index] = imported

		return nil
	}
}

func (preparation *mediaPreparation) importedPaths() []string {
	imported := make([]string, zero, len(preparation.paths))
	for index := range preparation.paths {
		if preparation.paths[index] != "" {
			imported = append(imported, preparation.paths[index])
		}
	}

	return imported
}

func indexItems(items []contentapi.Item) map[string]contentapi.Item {
	result := make(map[string]contentapi.Item, len(items))
	for index := range items {
		result[items[index].ID] = items[index]
	}

	return result
}

func (preparation *itemPreparation) prepareAll(
	drafts []contentapi.Draft,
) ([]contentapi.Item, error) {
	items := make([]contentapi.Item, zero, len(drafts))
	for index := range drafts {
		item, err := preparation.prepare(&drafts[index], index)
		if err != nil {
			return nil, fmt.Errorf("prepare item: %w", err)
		}

		items = append(items, item)
	}

	if len(items) == zero {
		return nil, errAddSomethingYouWantToRemember
	}

	return items, nil
}

func (preparation *itemPreparation) prepare(
	draft *contentapi.Draft,
	position int,
) (contentapi.Item, error) {
	item, err := preparation.resolve(draft)
	if err != nil {
		return contentapi.Item{}, fmt.Errorf("resolve draft: %w", err)
	}

	item.Position = position

	preparation.seen[item.ID] = true

	return item, nil
}

func (preparation *itemPreparation) resolve(draft *contentapi.Draft) (contentapi.Item, error) {
	resolve := preparation.create

	if draft.ID != "" {
		resolve = preparation.retain
	}

	item, err := resolve(draft)
	if err != nil {
		return contentapi.Item{}, fmt.Errorf("resolve draft content: %w", err)
	}

	return item, nil
}

func (preparation *itemPreparation) retain(draft *contentapi.Draft) (contentapi.Item, error) {
	item, exists := preparation.previous[draft.ID]
	if !exists || preparation.seen[draft.ID] {
		return contentapi.Item{}, errInvalidExistingItemOrOwnership
	}

	if item.Kind != draft.Kind || draft.SourcePath != "" {
		return contentapi.Item{}, errInvalidExistingItemOrOwnership
	}

	return item, nil
}

func (preparation *itemPreparation) create(draft *contentapi.Draft) (contentapi.Item, error) {
	id, err := preparation.newID()
	if err != nil {
		return contentapi.Item{}, fmt.Errorf("create item identifier: %w", err)
	}

	return newDraftItem(id, draft.Kind), nil
}

func newDraftItem(id, kind string) contentapi.Item {
	var item contentapi.Item

	item.ID = id
	item.Kind = kind

	return item
}

func (rules *Rules) applyDraft(item *contentapi.Item, draft *contentapi.Draft) error {
	err := rules.applyDraftContent(item, draft)
	if err != nil {
		return fmt.Errorf("apply content: %w", err)
	}

	err = rules.ValidateItem(item)
	if err != nil {
		return fmt.Errorf("validate prepared content: %w", err)
	}

	return nil
}

func (rules *Rules) applyDraftContent(item *contentapi.Item, draft *contentapi.Draft) error {
	if draft.Kind != rules.TextKind && draft.Text == "" {
		return nil
	}

	if draft.Kind != rules.TextKind {
		return errMediaCannotContainText
	}

	err := applyTextDraft(item, draft)
	if err != nil {
		return fmt.Errorf("apply draft content: %w", err)
	}

	return nil
}

func applyTextDraft(item *contentapi.Item, draft *contentapi.Draft) error {
	if draft.SourcePath != "" {
		return errTextCannotReferenceAFile
	}

	item.Text = strings.TrimSpace(draft.Text)

	return nil
}
