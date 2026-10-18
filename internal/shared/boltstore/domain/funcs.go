// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package domain

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	reviewapi "raven/api/reviews"
)

// ValidateItem checks structural and kind-specific content invariants.
func ValidateItem(rules *Rules, item *contentapi.Item) error {
	if item.ID == "" || item.Position < zero {
		return errInvalidContentItem
	}

	err := validateKind(rules, item)
	if err != nil {
		return fmt.Errorf("validate content item: %w", err)
	}

	return nil
}

func validateKind(rules *Rules, item *contentapi.Item) error {
	var err error

	if item.Kind == rules.TextKind {
		err = validateText(item)
	}

	if item.Kind != rules.TextKind {
		err = validateMedia(rules, item)
	}

	if err != nil {
		return fmt.Errorf("validate content kind: %w", err)
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

func validateMedia(rules *Rules, item *contentapi.Item) error {
	if !isMediaKind(item.Kind) {
		return errInvalidContentKind
	}

	if !validMediaMetadata(item) {
		return errInvalidMediaItem
	}

	err := validateMediaLocation(rules, item)
	if err != nil {
		return fmt.Errorf("validate media: %w", err)
	}

	return nil
}

func validMediaMetadata(item *contentapi.Item) bool {
	return item.Text == "" && item.OriginalName != "" && item.ByteSize > zero
}

func isMediaKind(kind string) bool { return kind == "audio" || kind == "image" || kind == "video" }

func validateMediaLocation(rules *Rules, item *contentapi.Item) error {
	if !mediaLocationMatches(rules, item) {
		return errInvalidMediaItem
	}

	return nil
}

func mediaLocationMatches(rules *Rules, item *contentapi.Item) bool {
	return strings.HasPrefix(item.MIMEType, item.Kind+"/") &&
		ValidMediaPath(rules, item.RelativePath, item.ID)
}

// ValidMediaPath checks that a supported media path belongs to the item identifier.
func ValidMediaPath(rules *Rules, path, id string) bool {
	if !itemIDPattern.MatchString(id) {
		return false
	}

	extension := filepath.Ext(path)

	return MediaType(rules, extension) != "" && path == "media/"+id+extension
}

// MediaType returns the supported MIME type for a lowercase file extension.
// Active document formats, including SVG and HTML, are deliberately unsupported.
func MediaType(rules *Rules, extension string) string {
	if mediaType := imageMediaType(rules, extension); mediaType != "" {
		return mediaType
	}

	if mediaType := audioMediaType(rules, extension); mediaType != "" {
		return mediaType
	}

	return videoMediaType(rules, extension)
}

func imageMediaType(rules *Rules, extension string) string {
	return map[string]string{
		".jpg": rules.MIMEJPEG, ".jpeg": rules.MIMEJPEG, ".png": "image/png",
		".gif": "image/gif", ".webp": "image/webp", ".avif": "image/avif",
	}[extension]
}

func audioMediaType(rules *Rules, extension string) string {
	return map[string]string{
		".mp3": "audio/mpeg", ".wav": "audio/wav", ".m4a": "audio/mp4",
		".ogg": rules.MIMEOggAudio, ".oga": rules.MIMEOggAudio, ".flac": "audio/flac",
	}[extension]
}

func videoMediaType(rules *Rules, extension string) string {
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
		contentapi.Items(card.Items),
		func(first, second contentapi.Item) int { return cmp.Compare(first.Position, second.Position) },
	)

	for position := range contentapi.Items(card.Items) {
		if contentapi.Items(card.Items)[position].Position != position {
			return errInvalidContentOrder
		}
	}

	if len(contentapi.Items(card.Items)) == zero {
		return errAddSomethingYouWantToRemember
	}

	return nil
}

func aggregateSchedule(card *cardapi.Card) {
	card.DueAt = zero
	card.LastReviewed = zero

	for index := range contentapi.Items(card.Items) {
		updateDue(card, &contentapi.Items(card.Items)[index])
		updateLastReview(card, contentapi.Reviews(contentapi.Items(card.Items)[index].Reviews))
	}
}

func updateDue(card *cardapi.Card, item *contentapi.Item) {
	dueMilli := contentapi.Schedule(item.Schedule).Due.UnixMilli()
	if card.DueAt == zero || dueMilli < card.DueAt {
		card.DueAt = dueMilli
	}
}

func updateLastReview(card *cardapi.Card, reviews []contentapi.Review) {
	for index := range reviews {
		review := &reviews[index]
		if !review.Automatic {
			card.LastReviewed = max(
				card.LastReviewed,
				contentapi.Log(review.Log).Review.UnixMilli(),
			)
		}
	}
}

// InitializeItem creates the initial schedule and automatic review record.
func InitializeItem(item *contentapi.Item, now time.Time,
	initialize func(time.Time) (json.RawMessage, json.RawMessage, error),
) error {
	schedule, log, err := initialize(now)
	if err != nil {
		return fmt.Errorf("initialize item schedule: %w", err)
	}

	item.Schedule = schedule
	item.Reviews = []contentapi.Review{{Log: log, Automatic: true}}
	item.LastNotifiedDue = zero

	return nil
}

// ApplyReview advances a schedule after a validated user rating.
func ApplyReview(
	item *contentapi.Item,
	attempt *reviewapi.Attempt,
	dependencies *ReviewDependencies,
) error {
	rules, err := contentapi.Decode[Rules](dependencies.Rules)
	if err != nil {
		return fmt.Errorf("decode review policy: %w", err)
	}

	if attempt.Rating < rules.MinimumRating || attempt.Rating > rules.MaximumRating {
		return errInvalidReviewRating
	}

	err = advanceReview(item, attempt, dependencies.Next)
	if err != nil {
		return fmt.Errorf("apply review: %w", err)
	}

	return nil
}

func advanceReview(item *contentapi.Item, attempt *reviewapi.Attempt,
	next func(any, time.Time, int) (json.RawMessage, json.RawMessage, error),
) error {
	current := contentapi.Schedule(item.Schedule)

	schedule, log, err := next(&current, attempt.At, attempt.Rating)
	if err != nil {
		return fmt.Errorf("advance item schedule: %w", err)
	}

	item.Schedule = schedule
	appendReview(item, log)

	return nil
}

func appendReview(item *contentapi.Item, log json.RawMessage) {
	item.Reviews = append(
		contentapi.Reviews(item.Reviews),
		contentapi.Review{Log: log, Automatic: false},
	)
}

// PrepareItems validates drafts and returns newly imported files for rollback.
func PrepareItems(
	ctx context.Context,
	input *PreparationInput,
	dependencies *PreparationDependencies,
) ([]contentapi.Item, []string, error) {
	media, err := newMediaPreparation(input, dependencies)
	if err != nil {
		return nil, nil, fmt.Errorf("prepare card content: %w", err)
	}

	result, imported, err := finishPreparation(ctx, media)
	if err != nil {
		return nil, imported, fmt.Errorf("finish card content: %w", err)
	}

	return result, imported, nil
}

func resolveDrafts(
	input *PreparationInput,
	dependencies *PreparationDependencies,
) ([]contentapi.Item, error) {
	preparation, err := newItemPreparation(input, dependencies)
	if err != nil {
		return nil, fmt.Errorf("prepare previous content: %w", err)
	}

	drafts, err := contentapi.Decode[[]contentapi.Draft](input.Drafts)
	if err != nil {
		return nil, fmt.Errorf("decode content drafts: %w", err)
	}

	items, err := prepareAll(preparation, drafts)
	if err != nil {
		return nil, fmt.Errorf("resolve content drafts: %w", err)
	}

	return items, nil
}

func newItemPreparation(
	input *PreparationInput,
	dependencies *PreparationDependencies,
) (*itemPreparation, error) {
	previous, err := contentapi.Decode[[]contentapi.Item](input.Previous)
	if err != nil {
		return nil, fmt.Errorf("decode previous content: %w", err)
	}

	return &itemPreparation{
		previous: indexItems(previous), seen: make(map[string]bool), newID: dependencies.NewID,
	}, nil
}

func newMediaPreparation(
	input *PreparationInput,
	dependencies *PreparationDependencies,
) (*mediaPreparation, error) {
	rules, err := contentapi.Decode[Rules](dependencies.Rules)
	if err != nil {
		return nil, fmt.Errorf("decode content policy: %w", err)
	}

	items, err := resolveDrafts(input, dependencies)
	if err != nil {
		return nil, fmt.Errorf("resolve preparation content: %w", err)
	}

	return &mediaPreparation{
		rules: rules, items: items, drafts: contentapi.Drafts(input.Drafts),
		batch: dependencies.Batch, importMedia: dependencies.ImportMedia,
		textKind: rules.TextKind, paths: []string{},
	}, nil
}

func finishPreparation(
	ctx context.Context,
	media *mediaPreparation,
) ([]contentapi.Item, []string, error) {
	rules, err := contentapi.Decode[Rules](media.rules)
	if err != nil {
		return nil, nil, fmt.Errorf("decode prepared policy: %w", err)
	}

	imported, err := importAll(ctx, media)
	if err != nil {
		return nil, imported, fmt.Errorf("import card content: %w", err)
	}

	err = applyDrafts(&rules, contentapi.Items(media.items), contentapi.Drafts(media.drafts))
	if err != nil {
		return nil, imported, fmt.Errorf("apply drafts: %w", err)
	}

	return contentapi.Items(media.items), imported, nil
}

func applyDrafts(rules *Rules, items []contentapi.Item, drafts []contentapi.Draft) error {
	for index := range items {
		err := applyDraft(rules, &items[index], &drafts[index])
		if err != nil {
			return fmt.Errorf("apply draft: %w", err)
		}
	}

	return nil
}

func importAll(ctx context.Context, preparation *mediaPreparation) ([]string, error) {
	jobs := jobs(preparation)
	err := preparation.batch(ctx, jobs)
	imported := importedPaths(preparation)

	if err != nil {
		return imported, fmt.Errorf("run media imports: %w", err)
	}

	return imported, nil
}

func jobs(preparation *mediaPreparation) []func(context.Context) error {
	jobs := make([]func(context.Context) error, zero, len(contentapi.Items(preparation.items)))
	for index := range contentapi.Items(preparation.items) {
		preparation.paths = append(preparation.paths, "")
		if needsImport(preparation, index) {
			jobs = append(jobs, importJob(preparation, index))
		}
	}

	return jobs
}

func needsImport(preparation *mediaPreparation, index int) bool {
	return contentapi.Drafts(preparation.drafts)[index].ID == "" &&
		contentapi.Items(preparation.items)[index].Kind != preparation.textKind
}

func importJob(preparation *mediaPreparation, index int) func(context.Context) error {
	return func(context.Context) error {
		imported, path, err := importItem(preparation, index)

		preparation.paths[index] = path

		if err != nil {
			return fmt.Errorf("import draft media: %w", err)
		}

		contentapi.Items(preparation.items)[index] = imported

		return nil
	}
}

func importedPaths(preparation *mediaPreparation) []string {
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

func prepareAll(preparation *itemPreparation,
	drafts []contentapi.Draft,
) ([]contentapi.Item, error) {
	items := make([]contentapi.Item, zero, len(drafts))
	for index := range drafts {
		item, err := prepare(preparation, &drafts[index], index)
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

func prepare(preparation *itemPreparation,
	draft *contentapi.Draft,
	position int,
) (contentapi.Item, error) {
	item, err := resolve(preparation, draft)
	if err != nil {
		return contentapi.Item{}, fmt.Errorf("resolve draft: %w", err)
	}

	item.Position = position

	preparation.seen[item.ID] = true

	return item, nil
}

func resolve(preparation *itemPreparation, draft *contentapi.Draft) (contentapi.Item, error) {
	resolveDraft := create

	if draft.ID != "" {
		resolveDraft = retain
	}

	item, err := resolveDraft(preparation, draft)
	if err != nil {
		return contentapi.Item{}, fmt.Errorf("resolve draft content: %w", err)
	}

	return item, nil
}

func retain(preparation *itemPreparation, draft *contentapi.Draft) (contentapi.Item, error) {
	item, exists := indexedItems(preparation.previous)[draft.ID]
	if !exists || preparation.seen[draft.ID] {
		return contentapi.Item{}, errInvalidExistingItemOrOwnership
	}

	if item.Kind != draft.Kind || draft.SourcePath != "" {
		return contentapi.Item{}, errInvalidExistingItemOrOwnership
	}

	return item, nil
}

func create(preparation *itemPreparation, draft *contentapi.Draft) (contentapi.Item, error) {
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

func applyDraft(rules *Rules, item *contentapi.Item, draft *contentapi.Draft) error {
	err := applyDraftContent(rules, item, draft)
	if err != nil {
		return fmt.Errorf("apply content: %w", err)
	}

	err = ValidateItem(rules, item)
	if err != nil {
		return fmt.Errorf("validate prepared content: %w", err)
	}

	return nil
}

func applyDraftContent(rules *Rules, item *contentapi.Item, draft *contentapi.Draft) error {
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

func indexedItems(value any) map[string]contentapi.Item {
	items, ok := value.(map[string]contentapi.Item)
	if !ok {
		return nil
	}

	return items
}

func importItem(preparation *mediaPreparation, index int) (contentapi.Item, string, error) {
	encoded, err := json.Marshal(contentapi.Items(preparation.items)[index])
	if err != nil {
		return contentapi.Item{}, "", fmt.Errorf("encode media content: %w", err)
	}

	imported, path, err := preparation.importMedia(
		encoded,
		contentapi.Drafts(preparation.drafts)[index].SourcePath,
	)
	if err != nil {
		return contentapi.Item{}, path, fmt.Errorf("import media content: %w", err)
	}

	item, err := contentapi.Decode[contentapi.Item](imported)
	if err != nil {
		return contentapi.Item{}, path, fmt.Errorf("decode imported content: %w", err)
	}

	return item, path, nil
}
