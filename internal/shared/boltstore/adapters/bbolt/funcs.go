// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package bbolt

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	bolt "go.etcd.io/bbolt"
	"go.uber.org/multierr"

	cardapi "raven/api/cards"
	contentapi "raven/api/content"
	folderapi "raven/api/folders"
	reviewapi "raven/api/reviews"
	library "raven/internal/shared/boltstore/domain"
	localfiles "raven/internal/shared/filesystem"
)

// ClaimAutostartInitialization records the attempt before touching OS settings.
// A missing bucket is normal for existing databases; no schema migration is needed.
func storeClaimAutostartInitialization(storage *Store) (bool, error) {
	claimed, err := storeClaimMarker(storage, claimLoginSetting)
	if err != nil {
		return false, fmt.Errorf("record login startup initialization: %w", err)
	}

	return claimed, nil
}

func claimLoginSetting(transaction *bolt.Tx) (bool, error) {
	settings, err := transaction.CreateBucketIfNotExists([]byte(settingsBucket))
	if err != nil {
		return false, fmt.Errorf(claimAutostartInitializationErrorFormat, err)
	}

	claimed, err := markOnce(settings, []byte(autostartInitializedKey), []byte("1"))
	if err != nil {
		return claimed, fmt.Errorf(claimAutostartInitializationErrorFormat, err)
	}

	return claimed, nil
}

// MarkNotified only acknowledges schedules that have not changed since delivery.
func storeMarkNotified(storage *Store, occurrences []reviewapi.DueOccurrence) error {
	err := storage.db.Update(func(transaction *bolt.Tx) error {
		err := storeMarkOccurrences(storage, transaction, occurrences)
		if err != nil {
			return fmt.Errorf(markNotifiedErrorFormat, err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf(markNotifiedErrorFormat, err)
	}

	return nil
}

func storeMarkOccurrences(storage *Store,
	transaction *bolt.Tx,
	occurrences []reviewapi.DueOccurrence,
) error {
	cards := make(map[[two]string]*notificationCard)

	for index := range occurrences {
		occurrence := &occurrences[index]

		card, err := storeNotificationCard(storage, transaction, occurrence, cards)
		if err != nil {
			return fmt.Errorf("load notification occurrence: %w", err)
		}

		err = notificationCardAcknowledge(card, occurrence)
		if err != nil {
			return fmt.Errorf("acknowledge notification occurrence: %w", err)
		}
	}

	return nil
}

func storeNotificationCard(storage *Store,
	transaction *bolt.Tx,
	occurrence *reviewapi.DueOccurrence,
	cards map[[two]string]*notificationCard,
) (*notificationCard, error) {
	key := [two]string{occurrence.FolderID, occurrence.CardID}
	if card := cards[key]; card != nil {
		return card, nil
	}

	card, err := storeLoadCard(storage, transaction, occurrence.FolderID, occurrence.CardID)
	if err != nil {
		return nil, fmt.Errorf("load notification card: %w", err)
	}

	cached := newNotificationCard(transaction, &card)

	cards[key] = cached

	return cached, nil
}

func newNotificationCard(transaction *bolt.Tx, card *cardapi.Card) *notificationCard {
	return &notificationCard{
		items: indexContentItems(contentapi.Items(card.Items)),
		bucket: transaction.Bucket([]byte(foldersBucket)).Bucket([]byte(card.FolderID)).
			Bucket([]byte(itemsBucket)).Bucket([]byte(card.ID)),
	}
}

func notificationCardAcknowledge(card *notificationCard, occurrence *reviewapi.DueOccurrence) error {
	item := card.items[occurrence.ItemID]
	if item == nil {
		return nil
	}

	if contentapi.Schedule(item.Schedule).Due.UnixMilli() != occurrence.Due ||
		item.LastNotifiedDue == occurrence.Due {

		return nil
	}

	item.LastNotifiedDue = occurrence.Due

	err := writeContentItem(card.bucket, item)
	if err != nil {
		return fmt.Errorf("persist notification acknowledgment: %w", err)
	}

	return nil
}

// ClaimNotificationPrompt persists the one-time authorization prompt decision.
func storeClaimNotificationPrompt(storage *Store) (bool, error) {
	claimed, err := storeClaimMarker(storage, claimNotificationSetting)
	if err != nil {
		return claimed, fmt.Errorf("claim notification prompt: %w", err)
	}

	return claimed, nil
}

func claimNotificationSetting(transaction *bolt.Tx) (bool, error) {
	metadata := transaction.Bucket([]byte(metaBucket))
	if metadata == nil {
		return false, errMissingMetadata
	}

	claimed, err := markOnce(metadata, []byte("notificationAuthorizationRequested"), []byte("true"))
	if err != nil {
		return claimed, fmt.Errorf("record notification prompt: %w", err)
	}

	return claimed, nil
}

func storeClaimMarker(storage *Store, claim func(*bolt.Tx) (bool, error)) (bool, error) {
	claimed := false

	err := storage.db.Update(func(transaction *bolt.Tx) error {
		var failure error

		claimed, failure = claim(transaction)
		if failure != nil {
			return fmt.Errorf("claim persistent marker: %w", failure)
		}

		return nil
	})
	if err != nil {
		return claimed, fmt.Errorf("commit persistent marker: %w", err)
	}

	return claimed, nil
}

func markOnce(bucket *bolt.Bucket, key, value []byte) (bool, error) {
	if bucket.Get(key) != nil {
		return false, nil
	}

	err := bucket.Put(key, value)
	if err != nil {
		return true, fmt.Errorf("write persistent marker: %w", err)
	}

	return true, nil
}

func storeInitialize(storage *Store, path string) error {
	migration, err := storePlanMigration(storage, path)
	if err != nil {
		return fmt.Errorf("prepare database migration: %w", err)
	}

	if migration.state.version == schemaV5 {
		return nil
	}

	migration.now = time.Now()

	err = storage.db.Update(func(transaction *bolt.Tx) error { return schemaMigrationApply(migration, transaction) })
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	return nil
}

func storePlanMigration(storage *Store, path string) (*schemaMigration, error) {
	state, err := storeReadSchema(storage)
	if err != nil {
		return nil, fmt.Errorf("inspect database schema: %w", err)
	}

	migration := &schemaMigration{storage: storage, state: state, now: time.Now()}
	if state.version == schemaV5 {
		return migration, nil
	}

	err = schemaMigrationBackup(migration, path)
	if err != nil {
		return nil, fmt.Errorf("back up database: %w", err)
	}

	return migration, nil
}

func storeReadSchema(storage *Store) (schemaState, error) {
	state := schemaState{version: schemaV1, existing: false}

	err := storage.db.View(func(transaction *bolt.Tx) error {
		state.version = readSchemaVersion(transaction)
		state.existing = transaction.Bucket([]byte(foldersBucket)) != nil

		return schemaStateValidate(&state)
	})
	if err != nil {
		return schemaState{}, fmt.Errorf("read database schema: %w", err)
	}

	return state, nil
}

func readSchemaVersion(transaction *bolt.Tx) string {
	metadata := transaction.Bucket([]byte(metaBucket))
	if metadata == nil {
		return schemaV1
	}

	version := metadata.Get([]byte(schemaKey))
	if len(version) == zero {
		return schemaV1
	}

	return string(version)
}

func schemaStateValidate(state *schemaState) error {
	if !slices.Contains([]string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5}, state.version) {
		return fmt.Errorf(errorFormat, errUnsupportedDatabaseSchema, state.version)
	}

	if state.version != schemaV1 && !state.existing {
		return errMissingFoldersBucket
	}

	return nil
}

func schemaMigrationBackup(migration *schemaMigration, path string) error {
	if !migration.state.existing {
		return nil
	}

	file, err := os.CreateTemp(
		filepath.Dir(path),
		filepath.Base(path)+".v"+migration.state.version+"-*.bak",
	)
	if err != nil {
		return fmt.Errorf("create migration backup: %w", err)
	}

	err = schemaMigrationFinishBackup(migration, file, filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("finish migration backup: %w", err)
	}

	return nil
}

func schemaMigrationFinishBackup(migration *schemaMigration, file *os.File, directory string) error {
	err := storeWriteBackup(migration.storage, file)
	if err != nil {
		return fmt.Errorf(
			"write migration backup: %w",
			multierr.Append(err, os.Remove(file.Name())),
		)
	}

	err = syncDirectory(directory)
	if err != nil {
		return fmt.Errorf("sync migration backup: %w", err)
	}

	return nil
}

func storeWriteBackup(storage *Store, file *os.File) error {
	err := storage.db.View(func(transaction *bolt.Tx) error {
		return writeBackupSnapshot(transaction, file)
	})
	if err == nil {
		err = file.Sync()
	}

	err = multierr.Append(err, file.Close())
	if err != nil {
		return fmt.Errorf("write and close migration backup: %w", err)
	}

	return nil
}

func syncAndClose(directory durableFile) error {
	err := multierr.Append(directory.Sync(), directory.Close())
	if err != nil {
		return fmt.Errorf("sync and close directory: %w", err)
	}

	return nil
}

func writeBackupSnapshot(transaction snapshot, file *os.File) error {
	written, err := transaction.WriteTo(file)
	if err != nil {
		return fmt.Errorf("copy migration snapshot: %w", err)
	}

	if written != transaction.Size() {
		return errIncompleteMigrationBackup
	}

	return nil
}

func schemaMigrationApply(migration *schemaMigration, transaction *bolt.Tx) error {
	metadata, err := transaction.CreateBucketIfNotExists([]byte(metaBucket))
	if err != nil {
		return fmt.Errorf("create migration metadata: %w", err)
	}

	err = schemaMigrationMigrateRoot(migration, transaction)
	if err != nil {
		return fmt.Errorf(migrateFolderRecordsErrorFormat, err)
	}

	err = metadata.Put([]byte(schemaKey), []byte(schemaV5))
	if err != nil {
		return fmt.Errorf("commit migrated schema version: %w", err)
	}

	return nil
}

func schemaMigrationMigrateRoot(migration *schemaMigration, transaction *bolt.Tx) error {
	root, err := transaction.CreateBucketIfNotExists([]byte(foldersBucket))
	if err != nil {
		return fmt.Errorf("create migration folder root: %w", err)
	}

	err = schemaMigrationMigrateFolders(migration, root)
	if err != nil {
		return fmt.Errorf(migrateFolderRecordsErrorFormat, err)
	}

	return nil
}

func schemaMigrationMigrateFolders(migration *schemaMigration, root *bolt.Bucket) error {
	err := root.ForEach(func(folderID, value []byte) error {
		err := schemaMigrationMigrateFolder(migration, root.Bucket(folderID), folderID, value)
		if err != nil {
			return fmt.Errorf("migrate folder: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("iterate legacy folders: %w", err)
	}

	return nil
}

func schemaMigrationMigrateFolder(migration *schemaMigration, folder *bolt.Bucket, folderID, value []byte) error {
	prepared, err := schemaMigrationPrepareFolder(migration, folder, value)
	if err != nil {
		return fmt.Errorf("prepare legacy folder: %w", err)
	}

	return errors.Join(
		schemaMigrationMigrateFolderCards(migration, prepared, folder.Bucket([]byte(cardsBucket)), folderID),
	)
}

func schemaMigrationMigrateFolderCards(migration *schemaMigration,
	prepared *migrationFolder,
	cards *bolt.Bucket,
	folderID []byte,
) error {
	ids, err := legacyCardIDs(cards)
	if err != nil {
		return fmt.Errorf("read legacy card identifiers: %w", err)
	}

	err = schemaMigrationMigrateCards(migration, prepared, folderID, ids)
	if err != nil {
		return fmt.Errorf("migrate legacy cards: %w", err)
	}

	return nil
}

func schemaMigrationPrepareFolder(migration *schemaMigration,
	folder *bolt.Bucket,
	value []byte,
) (*migrationFolder, error) {
	err := schemaMigrationCheckFolderLayout(migration, folder, value)
	if err != nil {
		return nil, fmt.Errorf("validate legacy folder: %w", err)
	}

	items, err := folder.CreateBucketIfNotExists([]byte(itemsBucket))
	if err != nil {
		return nil, fmt.Errorf("create legacy content root: %w", err)
	}

	return &migrationFolder{parent: folder, items: items}, nil
}

func schemaMigrationCheckFolderLayout(migration *schemaMigration, folder *bolt.Bucket, value []byte) error {
	if !validFolderEntry(folder, value) {
		return errInvalidLegacyFolder
	}

	if migration.state.version == schemaV2 && folder.Bucket([]byte(itemsBucket)) == nil {
		return errMissingLegacyContentBucket
	}

	return nil
}

func validFolderEntry(folder *bolt.Bucket, value []byte) bool {
	return value == nil && folder != nil && folder.Bucket([]byte(cardsBucket)) != nil
}

func legacyCardIDs(bucket *bolt.Bucket) ([][]byte, error) {
	ids := [][]byte{}
	// Collect keys before any writes can move the bucket cursor.
	err := bucket.ForEach(
		func(id, _ []byte) error {
			ids = append(ids, bytes.Clone(id))

			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("collect legacy card identifiers: %w", err)
	}

	return ids, nil
}

func schemaMigrationMigrateCards(migration *schemaMigration,
	folder *migrationFolder,
	folderID []byte,
	ids [][]byte,
) error {
	for index := range ids {
		err := schemaMigrationMigrateCard(migration, folder, folderID, ids[index])
		if err != nil {
			return fmt.Errorf("migrate legacy card: %w", err)
		}
	}

	return nil
}

func schemaMigrationMigrateCard(migration *schemaMigration,
	folder *migrationFolder,
	folderID, cardID []byte,
) error {
	card, err := readLegacyCard(folder.parent.Bucket([]byte(cardsBucket)), folderID, cardID)
	if err != nil {
		return fmt.Errorf("read legacy metadata: %w", err)
	}

	err = schemaMigrationPrepareCardContent(migration, folder.parent, &card)
	if err != nil {
		return fmt.Errorf("prepare legacy content: %w", err)
	}

	err = schemaMigrationPersistCard(migration, folder, &card)
	if err != nil {
		return fmt.Errorf("persist migrated card: %w", err)
	}

	return nil
}

func schemaMigrationPrepareCardContent(migration *schemaMigration,
	folder *bolt.Bucket,
	card *cardapi.Card,
) error {
	err := schemaMigrationLoadLegacyContent(migration, folder, card)
	if err != nil {
		return fmt.Errorf("read legacy content: %w", err)
	}

	err = schemaMigrationInitializeSchedules(migration, card)
	if err != nil {
		return fmt.Errorf("initialize legacy schedules: %w", err)
	}

	return nil
}

func readLegacyCard(bucket *bolt.Bucket, folderID, cardID []byte) (cardapi.Card, error) {
	var card cardapi.Card

	err := decodeRecord(bucket.Get(cardID), &card)
	if err != nil {
		return card, fmt.Errorf("decode legacy card metadata: %w", err)
	}

	if card.ID != string(cardID) || card.FolderID != string(folderID) {
		return cardapi.Card{}, fmt.Errorf(errorFormat, errInvalidLegacyCardOwnership, cardID)
	}

	card.Items = []contentapi.Item{}

	return card, nil
}

func schemaMigrationLoadLegacyContent(migration *schemaMigration, folder *bolt.Bucket, card *cardapi.Card) error {
	load := func(
		folder *bolt.Bucket,
		card *cardapi.Card,
	) error {
		return schemaMigrationLoadUnscheduledContent(migration, folder, card)
	}
	if migration.state.version == schemaV3 || migration.state.version == schemaV4 {
		load = func(
			folder *bolt.Bucket,
			card *cardapi.Card,
		) error {
			return schemaMigrationLoadScheduledFolderContent(migration, folder, card)
		}
	}

	err := load(folder, card)
	if err != nil {
		return fmt.Errorf("load legacy content records: %w", err)
	}

	return nil
}

func schemaMigrationLoadUnscheduledContent(migration *schemaMigration,
	folder *bolt.Bucket,
	card *cardapi.Card,
) error {
	items, err := schemaMigrationUnscheduledItems(migration, folder, card)
	if err != nil {
		return fmt.Errorf("load unscheduled content records: %w", err)
	}

	card.Items = items

	return nil
}

func schemaMigrationUnscheduledItems(migration *schemaMigration,
	folder *bolt.Bucket,
	card *cardapi.Card,
) ([]contentapi.Item, error) {
	load, err := schemaMigrationLegacyLoader(migration)
	if err != nil {
		return nil, fmt.Errorf("select legacy content reader: %w", err)
	}

	items, err := load(migration.storage.settings, folder, card)
	if err != nil {
		return nil, fmt.Errorf("read unscheduled content: %w", err)
	}

	return items, nil
}

func schemaMigrationLegacyLoader(migration *schemaMigration) (legacyContentLoader, error) {
	switch migration.state.version {
	case schemaV1:
		return legacyTextContent, nil
	case schemaV2:
		return legacyOrderedContent, nil
	default:
		return nil, errUnsupportedDatabaseSchema
	}
}

func legacyTextContent(
	settings *Settings,
	folder *bolt.Bucket,
	card *cardapi.Card,
) ([]contentapi.Item, error) {
	items, err := legacyTextItems(settings, folder.Bucket([]byte(cardsBucket)).Get([]byte(card.ID)))
	if err != nil {
		return nil, fmt.Errorf("read legacy text: %w", err)
	}

	return items, nil
}

func legacyOrderedContent(
	settings *Settings,
	folder *bolt.Bucket,
	card *cardapi.Card,
) ([]contentapi.Item, error) {
	items, err := mergeLegacyItems(
		settings,
		folder.Bucket([]byte(itemsBucket)).Bucket([]byte(card.ID)),
		card,
	)
	if err != nil {
		return nil, fmt.Errorf("read legacy ordered content: %w", err)
	}

	return items, nil
}

func schemaMigrationLoadScheduledFolderContent(migration *schemaMigration,
	folder *bolt.Bucket,
	card *cardapi.Card,
) error {
	err := schemaMigrationLoadScheduledContent(migration, folder.Tx(), card)
	if err != nil {
		return fmt.Errorf("load scheduled folder content: %w", err)
	}

	return nil
}

func schemaMigrationLoadScheduledContent(migration *schemaMigration,
	transaction *bolt.Tx,
	card *cardapi.Card,
) error {
	previous, err := storeLoadCard(migration.storage, transaction, card.FolderID, card.ID)
	if err != nil {
		return fmt.Errorf("read scheduled legacy content: %w", err)
	}

	card.Items = contentapi.Items(previous.Items)

	return nil
}

func legacyTextItems(settings *Settings, data []byte) ([]contentapi.Item, error) {
	var previous legacyText

	err := decodeRecord(data, &previous)
	if err != nil {
		return nil, fmt.Errorf("decode legacy text: %w", err)
	}

	if strings.TrimSpace(previous.Prompt) == "" || strings.TrimSpace(previous.Answer) == "" {
		return nil, errInvalidLegacyCard
	}

	items, err := createLegacyTextItems(settings, []string{previous.Prompt, previous.Answer})
	if err != nil {
		return nil, fmt.Errorf("create legacy text content: %w", err)
	}

	return items, nil
}

func createLegacyTextItems(settings *Settings, texts []string) ([]contentapi.Item, error) {
	items := make([]contentapi.Item, zero, len(texts))
	for position := range texts {
		item, err := legacyTextItem(settings, texts[position], position)
		if err != nil {
			return nil, fmt.Errorf("create legacy text item: %w", err)
		}

		items = append(items, item)
	}

	return items, nil
}

func legacyTextItem(settings *Settings, text string, position int) (contentapi.Item, error) {
	var item contentapi.Item

	id, err := newItemID()
	if err != nil {
		return item, fmt.Errorf("create legacy item identifier: %w", err)
	}

	item.ID = id
	item.Kind = settings.Rules.TextKind
	item.Position = position
	item.Text = text

	return item, nil
}

func schemaMigrationInitializeSchedules(migration *schemaMigration, card *cardapi.Card) error {
	if migration.state.version == schemaV4 {
		return nil
	}

	for index := range contentapi.Items(card.Items) {
		err := library.InitializeItem(
			&contentapi.Items(card.Items)[index],
			migration.now,
			migration.storage.scheduler,
		)
		if err != nil {
			return fmt.Errorf("initialize migrated item: %w", err)
		}
	}

	return nil
}

func schemaMigrationPersistCard(migration *schemaMigration, folder *migrationFolder, card *cardapi.Card) error {
	assignContentOwnership(card)

	err := writeCardBuckets(folder.parent.Bucket([]byte(cardsBucket)), folder.items, card)
	if err != nil {
		return fmt.Errorf("write migrated card: %w", err)
	}

	err = schemaMigrationVerifyCard(migration, folder.parent.Tx(), card)
	if err != nil {
		return fmt.Errorf("verify migrated card: %w", err)
	}

	return nil
}

func schemaMigrationVerifyCard(migration *schemaMigration, transaction *bolt.Tx, card *cardapi.Card) error {
	loaded, err := storeLoadCard(migration.storage, transaction, card.FolderID, card.ID)
	if err != nil {
		return fmt.Errorf("read migrated card: %w", err)
	}

	if len(contentapi.Items(loaded.Items)) != len(contentapi.Items(card.Items)) {
		return errMissingCardContent
	}

	return nil
}

// Merge prompt items followed by answer items, retaining every item ID and file.
func mergeLegacyItems(
	settings *Settings,
	bucket *bolt.Bucket,
	card *cardapi.Card,
) ([]contentapi.Item, error) {
	if bucket == nil {
		return nil, errMissingLegacyCardContent
	}

	old, err := collectLegacyItems(settings, bucket, card)
	if err != nil {
		return nil, fmt.Errorf(mergeLegacyItemsErrorFormat, err)
	}

	items, err := mergeOrderedItems(old)
	if err != nil {
		return nil, fmt.Errorf(mergeLegacyItemsErrorFormat, err)
	}

	return items, nil
}

func collectLegacyItems(
	settings *Settings,
	bucket *bolt.Bucket,
	card *cardapi.Card,
) ([]*legacyItem, error) {
	old := []*legacyItem{}

	err := bucket.ForEach(settingsLegacyCollector(settings, card, &old))
	if err != nil {
		return nil, fmt.Errorf(mergeLegacyItemsErrorFormat, err)
	}

	return old, nil
}

func settingsReadLegacyItem(settings *Settings, id, data []byte, card *cardapi.Card) (*legacyItem, error) {
	item := &legacyItem{Item: new(contentapi.Item), Side: ""}

	err := decodeRecord(data, item)
	if err != nil {
		return nil, fmt.Errorf("decode legacy item: %w", err)
	}

	if !contentBelongsTo(card, item.Item, string(id)) || !validLegacySide(item.Side) {
		return nil, errInvalidLegacyContentOwnershipOrSide
	}

	err = library.ValidateItem(&settings.Rules, item.Item)
	if err != nil {
		return nil, fmt.Errorf("validate legacy content: %w", err)
	}

	return item, nil
}

func contentBelongsTo(card *cardapi.Card, item *contentapi.Item, id string) bool {
	return item.ID == id && item.CardID == card.ID && item.FolderID == card.FolderID
}

func validLegacySide(
	side string,
) bool {
	return side == legacyPromptSide || side == legacyAnswerSide
}

func compareLegacyItems(first, second *legacyItem) int {
	if first.Side == second.Side {
		return cmp.Compare(first.Position, second.Position)
	}

	if first.Side == legacyPromptSide {
		return -one
	}

	return one
}

func mergeOrderedItems(old []*legacyItem) ([]contentapi.Item, error) {
	slices.SortFunc(old, compareLegacyItems)

	err := validateLegacyOrder(old)
	if err != nil {
		return nil, fmt.Errorf("order legacy content: %w", err)
	}

	items := make([]contentapi.Item, zero, len(old))
	for position := range old {
		item := *old[position].Item

		item.Position = position
		items = append(items, item)
	}

	return items, nil
}

func validateLegacyOrder(old []*legacyItem) error {
	counts := map[string]int{legacyPromptSide: zero, legacyAnswerSide: zero}

	for position := range old {
		item := old[position].Item
		side := old[position].Side

		if item.Position != counts[side] {
			return errInvalidLegacyContentOrder
		}

		counts[side]++
	}

	if counts[legacyPromptSide] == zero || counts[legacyAnswerSide] == zero {
		return errMissingLegacyCardContent
	}

	return nil
}

func storeMediaPath(storage *Store, relative string) string {
	return filepath.Join(storage.mediaDir, filepath.Base(relative))
}

func settingsInspectMedia(settings *Settings, file *os.File, kind, source string) (string, error) {
	declared := library.MediaType(&settings.Rules, strings.ToLower(filepath.Ext(source)))
	if !supportedMediaKind(kind) || !strings.HasPrefix(declared, kind+"/") {
		return "", errChooseASupportedFileForThisMedia
	}

	header, err := readMediaHeader(file)
	if err != nil {
		return "", fmt.Errorf("read selected media header: %w", err)
	}

	if !compatibleMedia(settings, declared, header) {
		return "", fmt.Errorf("%w %s", errMediaContentTypeMismatch, declared)
	}

	return declared, nil
}

func supportedMediaKind(kind string) bool {
	return kind == "audio" || kind == "image" || kind == "video"
}

func readMediaHeader(file *os.File) ([]byte, error) {
	var header [mediaHeaderSize]byte

	count, err := file.ReadAt(header[:], zero)

	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read media sample: %w", err)
	}

	if count == zero {
		return nil, errEmptyMediaFile
	}

	return header[:count], nil
}

func compatibleMedia(settings *Settings, declared string, header []byte) bool {
	detected := http.DetectContentType(header)

	return detected == declared || detected == mediaAlias(settings, declared) ||
		compatibleSignature(declared, header)
}

func mediaAlias(settings *Settings, declared string) string {
	switch declared {
	case "audio/wav":
		return "audio/wave"
	case settings.Rules.MIMEOggAudio, "video/ogg":
		return "application/ogg"
	case "audio/mp4":
		return settings.Rules.MIMEMP4Video
	default:
		return ""
	}
}

func compatibleSignature(declared string, header []byte) bool {
	switch declared {
	case "video/quicktime":
		return isoMediaHeader(header)
	case "image/avif":
		return avifHeader(header)
	case "audio/flac":
		return bytes.HasPrefix(header, []byte("fLaC"))
	default:
		return false
	}
}

func isoMediaHeader(header []byte) bool {
	return len(header) >= isoHeaderSize &&
		string(header[four:isoSignatureEnd]) == isoMediaSignature
}

func avifHeader(header []byte) bool {
	if !isoMediaHeader(header) {
		return false
	}

	brand := string(header[isoSignatureEnd:isoHeaderSize])

	return brand == "avif" || brand == "avis"
}

func storeImportMedia(storage *Store,
	item *contentapi.Item,
	source string,
) (contentapi.Item, string, error) {
	file, err := localfiles.OpenSelected(source)
	if err != nil {
		return *item, "", fmt.Errorf("open selected file: %w", err)
	}
	defer storeCloseMediaFile(storage, context.Background(), file)

	imported, path, err := storeImportOpenedMedia(storage, file, item, source)
	if err != nil {
		return imported, path, fmt.Errorf("import selected media: %w", err)
	}

	return imported, path, nil
}

func storeImportOpenedMedia(storage *Store,
	file *os.File,
	item *contentapi.Item,
	source string,
) (contentapi.Item, string, error) {
	imported := *item

	err := settingsInspectImportedMedia(storage.settings, file, source, &imported)
	if err != nil {
		return *item, "", fmt.Errorf(importMediaErrorFormat, err)
	}

	path, err := storePersistMedia(storage, file, &imported)
	if err != nil {
		return *item, "", fmt.Errorf(importMediaErrorFormat, err)
	}

	return imported, path, nil
}

func settingsInspectImportedMedia(settings *Settings, file *os.File,
	source string,
	item *contentapi.Item,
) error {
	size, err := regularMediaSize(file)
	if err != nil {
		return fmt.Errorf("inspect selected file: %w", err)
	}

	mimeType, err := settingsInspectMedia(settings, file, item.Kind, source)
	if err != nil {
		return fmt.Errorf("inspect media content: %w", err)
	}

	applyMediaMetadata(item, source, mimeType)

	item.ByteSize = size

	return nil
}

func applyMediaMetadata(item *contentapi.Item, source, mimeType string) {
	item.RelativePath = "media/" + item.ID + strings.ToLower(filepath.Ext(source))
	item.OriginalName = filepath.Base(source)
	item.MIMEType = mimeType
}

func regularMediaSize(file *os.File) (int64, error) {
	info, err := file.Stat()
	if err != nil {
		return zero, fmt.Errorf("inspect media file: %w", err)
	}

	if !info.Mode().IsRegular() {
		return zero, errSelectARegularMediaFile
	}

	return info.Size(), nil
}

func storePersistMedia(storage *Store, file *os.File, item *contentapi.Item) (string, error) {
	temp, err := os.CreateTemp(storage.mediaDir, ".import-*")
	if err != nil {
		return "", fmt.Errorf("create media staging file: %w", err)
	}
	defer storeRemoveMediaFile(storage, temp.Name())

	path, err := storeStageAndCommitMedia(storage, temp, file, item)
	if err != nil {
		return path, fmt.Errorf("persist staged media: %w", err)
	}

	return path, nil
}

func storeStageAndCommitMedia(storage *Store,
	temp, file *os.File,
	item *contentapi.Item,
) (string, error) {
	err := stageMediaFile(temp, file, item.ByteSize)
	if err != nil {
		return "", fmt.Errorf("stage media file: %w", err)
	}

	path, err := storeCommitMedia(storage, temp.Name(), storeMediaPath(storage, item.RelativePath))
	if err != nil {
		return "", fmt.Errorf("commit imported media: %w", err)
	}

	return path, nil
}

func stageMediaFile(target, source *os.File, expectedSize int64) error {
	size, err := copyMediaFile(target, source)
	if err != nil {
		return fmt.Errorf("copy media to staging: %w", err)
	}

	if size != expectedSize {
		return errSelectedFileChangedDuringImportTryAgain
	}

	return nil
}

func copyMediaFile(target *os.File, source io.Reader) (int64, error) {
	size, err := io.Copy(target, source)
	if err == nil {
		err = target.Sync()
	}

	err = multierr.Append(err, target.Close())
	if err != nil {
		return size, fmt.Errorf("copy and sync media: %w", err)
	}

	return size, nil
}

func storeCommitMedia(storage *Store, staged, path string) (string, error) {
	err := os.Rename(staged, path)
	if err != nil {
		return "", fmt.Errorf("rename imported media: %w", err)
	}

	err = syncDirectory(storage.mediaDir)
	if err != nil {
		storeRemoveMediaFile(storage, path)

		return "", fmt.Errorf("sync media directory: %w", err)
	}

	return path, nil
}

func storeCleanupMedia(storage *Store) error {
	// Validate all referenced records before removing anything.
	cards, err := storeListCards(storage)
	if err != nil {
		return fmt.Errorf(cleanupMediaErrorFormat, err)
	}

	err = os.MkdirAll(storage.mediaDir, privateDirectoryMode)
	if err != nil {
		return fmt.Errorf(cleanupMediaErrorFormat, err)
	}

	return errors.Join(storeRemoveUnreferencedMedia(storage, referencedMedia(storage.settings, cards)))
}

func referencedMedia(settings *Settings, cards []cardapi.Card) map[string]bool {
	referenced := make(map[string]bool)

	for index := range cards {
		referenceCardMedia(settings, &cards[index], referenced)
	}

	return referenced
}

func referenceCardMedia(settings *Settings, card *cardapi.Card, referenced map[string]bool) {
	for index := range contentapi.Items(card.Items) {
		item := &contentapi.Items(card.Items)[index]
		if item.Kind != settings.Rules.TextKind {
			referenced[filepath.Base(item.RelativePath)] = true
		}
	}
}

func storeRemoveUnreferencedMedia(storage *Store, referenced map[string]bool) error {
	entries, err := os.ReadDir(storage.mediaDir)
	if err != nil {
		return fmt.Errorf(cleanupMediaErrorFormat, err)
	}

	return errors.Join(storeCleanMediaEntries(storage, entries, referenced))
}

func storeCleanMediaEntries(storage *Store, entries []os.DirEntry, referenced map[string]bool) error {
	for index := range entries {
		err := storeCleanMediaEntry(storage, entries[index], referenced)
		if err != nil {
			return fmt.Errorf(cleanupMediaErrorFormat, err)
		}
	}

	return nil
}

func storeCleanMediaEntry(storage *Store, entry os.DirEntry, referenced map[string]bool) error {
	if !removableMedia(storage.settings, entry, referenced) {
		return nil
	}

	err := os.Remove(filepath.Join(storage.mediaDir, entry.Name()))
	if err != nil {
		return fmt.Errorf("remove unreferenced media: %w", err)
	}

	return nil
}

func removableMedia(settings *Settings, entry os.DirEntry, referenced map[string]bool) bool {
	name := entry.Name()
	managed := strings.HasPrefix(name, ".import-") ||
		library.ValidMediaPath(
			&settings.Rules,
			"media/"+name,
			strings.TrimSuffix(name, filepath.Ext(name)),
		)

	return !entry.IsDir() && managed && !referenced[name]
}

// MediaHandler serves only media records owned by the requested card.
// ServeContent implements HEAD and byte ranges without buffering the file.
func storeMediaHandler(storage *Store) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if !storage.ready.Load() {
			http.Error(writer, "media storage is unavailable", http.StatusServiceUnavailable)

			return
		}

		storeServeMedia(storage, writer, request)
	}
}

func storeServeMedia(storage *Store, writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.WriteHeader(http.StatusMethodNotAllowed)

		return
	}

	parts := strings.Split(
		strings.TrimPrefix(request.URL.Path, storage.settings.MediaRoutePrefix),
		"/",
	)
	if !validMediaRoute(storage.settings, request.URL.Path, parts) {
		http.NotFound(writer, request)

		return
	}

	storeServeOwnedMedia(storage, writer, request, parts)
}

func validMediaRoute(settings *Settings, path string, parts []string) bool {
	return strings.HasPrefix(path, settings.MediaRoutePrefix) && len(parts) == mediaRouteParts &&
		itemIDPattern.MatchString(parts[two])
}

func storeServeOwnedMedia(storage *Store,
	writer http.ResponseWriter,
	request *http.Request,
	parts []string,
) {
	file, item, err := storeOpenOwnedMedia(storage, parts)
	if err != nil {
		http.NotFound(writer, request)

		return
	}
	defer storeCloseMediaFile(storage, request.Context(), file)

	media := ownedMedia{file: file, item: &item}
	ownedMediaServe(&media, writer, request)
}

func ownedMediaServe(media *ownedMedia, writer http.ResponseWriter, request *http.Request) {
	info, err := media.file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(writer, request)

		return
	}

	setMediaHeaders(writer.Header(), media.item.MIMEType)
	http.ServeContent(writer, request, media.item.OriginalName, info.ModTime(), media.file)
}

func setMediaHeaders(header http.Header, mimeType string) {
	header.Set("Content-Type", mimeType)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Cache-Control", "no-store")
}

func storeOpenOwnedMedia(storage *Store, parts []string) (*os.File, contentapi.Item, error) {
	storage.mediaMu.Lock()
	defer storage.mediaMu.Unlock()

	item, err := storeReadOwnedMedia(storage, parts)
	if err != nil {
		return nil, item, fmt.Errorf(loadMediaErrorFormat, err)
	}

	file, err := os.OpenInRoot(storage.mediaDir, filepath.Base(item.RelativePath))
	if err != nil {
		return nil, item, fmt.Errorf("open saved media: %w", err)
	}

	return file, item, nil
}

func storeReadOwnedMedia(storage *Store, parts []string) (contentapi.Item, error) {
	var item contentapi.Item

	err := storage.db.View(func(transaction *bolt.Tx) error {
		var lookupErr error

		item, lookupErr = storeLookupMedia(storage, transaction, parts)
		if lookupErr != nil {
			return fmt.Errorf("look up owned media: %w", lookupErr)
		}

		return nil
	})
	if err != nil {
		return item, fmt.Errorf(loadMediaErrorFormat, err)
	}

	return item, nil
}

func storeLookupMedia(storage *Store,
	transaction *bolt.Tx,
	parts []string,
) (contentapi.Item, error) {
	card, err := storeLoadCard(storage, transaction, parts[zero], parts[one])
	if err != nil {
		return contentapi.Item{}, fmt.Errorf("media handler: %w", err)
	}

	for index := range contentapi.Items(card.Items) {
		item := &contentapi.Items(card.Items)[index]
		if item.ID == parts[two] && item.Kind != storage.settings.Rules.TextKind {
			return *item, nil
		}
	}

	return contentapi.Item{}, errMediaNotFound
}

// DefaultPath returns the application database path in the user configuration directory.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate Raven configuration directory: %w", err)
	}

	return filepath.Join(dir, "raven", "raven.db"), nil
}

// Open opens and migrates library persistence, then reconciles saved media.
func Open(path string, deps *Dependencies) (*Store, error) {
	storage := New(path, deps)

	err := storeStart(storage)
	if err != nil {
		return nil, fmt.Errorf("start library persistence: %w", err)
	}

	return storage, nil
}

// New constructs persistence without acquiring the database lock.
func New(path string, deps *Dependencies) *Store {
	storage := newStore(nil, path, deps)

	storage.logger = storageLogger([]*slog.Logger{deps.Logger})

	return storage
}

// Start opens, migrates, and reconciles persistence before consumers begin work.
func storeStart(storage *Store) error {
	storage.mediaMu.Lock()
	defer storage.mediaMu.Unlock()

	err := storeOpenPersistence(storage)
	if err != nil {
		return fmt.Errorf("start persistence: %w", err)
	}

	return nil
}

func storeOpenPersistence(storage *Store) error {
	if storage.db != nil {
		return errStoreAlreadyStarted
	}

	db, err := openDatabase(storage.path)
	if err != nil {
		return fmt.Errorf("open library persistence %q: %w", storage.path, err)
	}

	storage.db = db

	err = storePrepareOpenedDatabase(storage)
	if err != nil {
		return fmt.Errorf("prepare opened database: %w", err)
	}

	return nil
}

func storePrepareOpenedDatabase(storage *Store) error {
	err := storePrepare(storage, storage.path)
	if err != nil {
		failure := multierr.Append(err, storage.db.Close())

		storage.db = nil

		return fmt.Errorf("prepare library persistence: %w", failure)
	}

	storage.ready.Store(true)

	return nil
}

func openDatabase(path string) (*bolt.DB, error) {
	err := os.MkdirAll(filepath.Dir(path), privateDirectoryMode)
	if err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	var options bolt.Options

	options.Timeout = time.Second

	db, err := bolt.Open(path, privateFileMode, &options)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	return db, nil
}

func newStore(db database, path string, deps *Dependencies) *Store {
	settings := deps.Settings

	return &Store{
		settings:  &settings,
		path:      path,
		ready:     atomic.Bool{},
		db:        db,
		scheduler: deps.Scheduler,
		batch:     deps.Batch,
		mediaDir:  filepath.Join(filepath.Dir(path), "media"),
		mediaMu:   sync.Mutex{},
		logger:    nil,
	}
}

func storageLogger(loggers []*slog.Logger) *slog.Logger {
	if len(loggers) > zero && loggers[zero] != nil {
		return loggers[zero]
	}

	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

func storePrepare(storage *Store, path string) error {
	err := storeInitialize(storage, path)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}

	err = storeCleanupMedia(storage)
	if err != nil {
		return fmt.Errorf("reconcile database media: %w", err)
	}

	return nil
}

func storeCloseMediaFile(storage *Store, ctx context.Context, file *os.File) {
	err := file.Close()
	if err != nil {
		storage.logger.WarnContext(
			ctx,
			"could not close media file",
			//nolint:sloglint // Log field names come from validated embedded configuration.
			slog.Any(storage.settings.LogErrorKey, err),
		)
	}
}

func storeRemoveMediaFile(storage *Store, path string) {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}

	storeReportCleanupFailure(storage, err)
}

func storeReportCleanupFailure(storage *Store, err error) {
	if err != nil {
		storage.logger.WarnContext(
			context.Background(),
			"could not clean up media",
			//nolint:sloglint // Log field names come from validated embedded configuration.
			slog.Any(storage.settings.LogErrorKey, err),
		)
	}
}

// Close closes library persistence after current media operations finish.
func storeClose(storage *Store) error {
	storage.ready.Store(false)
	storage.mediaMu.Lock()
	defer storage.mediaMu.Unlock()

	if storage.db == nil {
		return nil
	}

	forwardErr2 := storage.db.Close()
	if forwardErr2 != nil {
		return fmt.Errorf("close: %w", forwardErr2)
	}

	return nil
}

// ListFolders returns folder metadata in creation order.
func storeListFolders(storage *Store) ([]folderapi.Folder, error) {
	reader := folderListing{root: nil, folders: []folderapi.Folder{}}

	err := storage.db.View(func(transaction *bolt.Tx) error { return folderListingRead(&reader, transaction) })
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}

	return reader.folders, nil
}

func folderListingRead(reader *folderListing, transaction *bolt.Tx) error {
	reader.root = transaction.Bucket([]byte(foldersBucket))
	if reader.root == nil {
		return errMissingFoldersBucket
	}

	err := reader.root.ForEach(func(key, value []byte) error { return folderListingAppendFolder(reader, key, value) })
	if err != nil {
		return fmt.Errorf("read folder records: %w", err)
	}

	return nil
}

func folderListingAppendFolder(reader *folderListing, key, value []byte) error {
	folder, err := readFolder(reader.root, key, value)
	if err != nil {
		return fmt.Errorf("read folder %q: %w", key, err)
	}

	reader.folders = append(reader.folders, folder)

	return nil
}

func readFolder(root *bolt.Bucket, key, value []byte) (folderapi.Folder, error) {
	bucket := root.Bucket(key)
	if !validFolderEntry(bucket, value) {
		return folderapi.Folder{}, fmt.Errorf(errorFormat, errInvalidFolderBucket, key)
	}

	folder, err := decodeFolder(bucket.Get([]byte(metadataKey)), string(key))
	if err != nil {
		return folderapi.Folder{}, fmt.Errorf("decode folder %q: %w", key, err)
	}

	return folder, nil
}

func decodeFolder(data []byte, id string) (folderapi.Folder, error) {
	var folder folderapi.Folder

	err := decodeRecord(data, &folder)
	if err != nil {
		return folder, fmt.Errorf("decode folder metadata: %w", err)
	}

	if folder.ID != id || strings.TrimSpace(folder.Title) == "" {
		return folderapi.Folder{}, fmt.Errorf(errorFormat, errInvalidFolderMetadata, id)
	}

	if folder.Tags == nil {
		folder.Tags = []string{}
	}

	return folder, nil
}

// CreateFolder persists normalized metadata in a new folder.
func storeCreateFolder(storage *Store, input *folderapi.CreateInput) (folderapi.Folder, error) {
	folder, err := library.NormalizeFolder(input)
	if err != nil {
		return folderapi.Folder{}, fmt.Errorf(createFolderErrorFormat, err)
	}

	err = storage.db.Update(func(transaction *bolt.Tx) error {
		persistErr := persistNewFolder(transaction, &folder)
		if persistErr != nil {
			return fmt.Errorf(createFolderErrorFormat, persistErr)
		}

		return nil
	})
	if err != nil {
		return folderapi.Folder{}, fmt.Errorf(createFolderErrorFormat, err)
	}

	return folder, nil
}

func persistNewFolder(transaction *bolt.Tx, folder *folderapi.Folder) error {
	root := transaction.Bucket([]byte(foldersBucket))
	if root == nil {
		return errMissingFoldersBucket
	}

	err := assignFolderID(root, folder)
	if err != nil {
		return fmt.Errorf("allocate folder: %w", err)
	}

	err = createFolderBucket(root, folder)
	if err != nil {
		return fmt.Errorf("persist folder bucket: %w", err)
	}

	return nil
}

func assignFolderID(root *bolt.Bucket, folder *folderapi.Folder) error {
	id, err := nextFolderID(root)
	if err != nil {
		return fmt.Errorf("assign folder identifier: %w", err)
	}

	folder.ID = id

	return nil
}

func createFolderBucket(root *bolt.Bucket, folder *folderapi.Folder) error {
	bucket, err := root.CreateBucket([]byte(folder.ID))
	if err != nil {
		return fmt.Errorf("create folder bucket: %w", err)
	}

	return errors.Join(initializeFolder(bucket, folder))
}

func nextFolderID(root *bolt.Bucket) (string, error) {
	sequence, err := root.NextSequence()
	if err != nil {
		return "", fmt.Errorf("allocate folder identifier: %w", err)
	}

	if sequence == zero {
		return "", errFolderIDSequenceExhausted
	}

	return fmt.Sprintf(recordIDFormat, sequence), nil
}

func initializeFolder(bucket *bolt.Bucket, folder *folderapi.Folder) error {
	err := writeFolderMetadata(bucket, folder)
	if err != nil {
		return fmt.Errorf("write folder metadata: %w", err)
	}

	names := [...]string{cardsBucket, itemsBucket}
	for index := range names {
		_, err = bucket.CreateBucket([]byte(names[index]))
		if err != nil {
			return fmt.Errorf("create folder content bucket: %w", err)
		}
	}

	return nil
}

func writeFolderMetadata(bucket *bolt.Bucket, folder *folderapi.Folder) error {
	return errors.Join(writeRecord(bucket, []byte(metadataKey), folder))
}

// DeleteCard deletes an owned card and reconciles its media files.
func storeDeleteCard(storage *Store, folderID, cardID string) error {
	storage.mediaMu.Lock()
	defer storage.mediaMu.Unlock()

	err := storage.db.Update(func(transaction *bolt.Tx) error {
		return storeDeleteCardRecords(storage, transaction, folderID, cardID)
	})
	if err != nil {
		return fmt.Errorf(deleteCardErrorFormat, err)
	}

	// Any failed file removal is retried by startup cleanup.
	storeReportCleanupFailure(storage, storeCleanupMedia(storage))

	return nil
}

func storeDeleteCardRecords(storage *Store, transaction *bolt.Tx, folderID, cardID string) error {
	card, err := storeLoadCard(storage, transaction, folderID, cardID)
	if err != nil {
		return fmt.Errorf(deleteCardErrorFormat, err)
	}

	folder := transaction.Bucket([]byte(foldersBucket)).Bucket([]byte(folderID))

	err = deleteOwnedCard(folder, card.ID)
	if err != nil {
		return fmt.Errorf("delete owned card records: %w", err)
	}

	return nil
}

func deleteOwnedCard(folder *bolt.Bucket, cardID string) error {
	err := folder.Bucket([]byte(itemsBucket)).DeleteBucket([]byte(cardID))
	if err != nil {
		return fmt.Errorf(deleteCardErrorFormat, err)
	}

	err = folder.Bucket([]byte(cardsBucket)).Delete([]byte(cardID))
	if err != nil {
		return fmt.Errorf("delete card metadata: %w", err)
	}

	return nil
}

// DeleteFolder deletes a folder and reconciles its media files.
func storeDeleteFolder(storage *Store, folderID string) error {
	storage.mediaMu.Lock()
	defer storage.mediaMu.Unlock()

	err := storage.db.Update(func(transaction *bolt.Tx) error {
		root := transaction.Bucket([]byte(foldersBucket))
		if root == nil || folderID == "" || root.Bucket([]byte(folderID)) == nil {
			return errFolderNotFound
		}

		return root.DeleteBucket([]byte(folderID))
	})
	if err != nil {
		return fmt.Errorf("delete folder: %w", err)
	}

	storeReportCleanupFailure(storage, storeCleanupMedia(storage))

	return nil
}

func newItemID() (string, error) {
	var id [16]byte

	count, readErr := io.ReadFull(rand.Reader, id[:])
	if readErr != nil {
		return "", fmt.Errorf("new item i d: %w", readErr)
	}

	return hex.EncodeToString(id[:count]), nil
}

func cardBucket(transaction *bolt.Tx, folderID string) (*bolt.Bucket, error) {
	err := validateDestinationFolder(transaction, folderID)
	if err != nil {
		return nil, fmt.Errorf("open card bucket: %w", err)
	}

	return transaction.Bucket([]byte(foldersBucket)).
			Bucket([]byte(folderID)).
			Bucket([]byte(cardsBucket)),
		nil
}

func validFolderContent(folder *bolt.Bucket) bool {
	return folder != nil && folder.Bucket([]byte(cardsBucket)) != nil &&
		folder.Bucket([]byte(itemsBucket)) != nil
}

func storeLoadCard(storage *Store,
	transaction *bolt.Tx,
	folderID, cardID string,
) (cardapi.Card, error) {
	bucket, err := cardBucket(transaction, folderID)
	if err != nil {
		return cardapi.Card{}, fmt.Errorf(loadCardErrorFormat, err)
	}

	card, err := readCardMetadata(bucket, folderID, cardID)
	if err != nil {
		return cardapi.Card{}, fmt.Errorf(loadCardErrorFormat, err)
	}

	hydrated, err := storeHydrateStoredCard(storage, transaction, &card)
	if err != nil {
		return hydrated, fmt.Errorf("hydrate stored card: %w", err)
	}

	return hydrated, nil
}

func storeHydrateStoredCard(storage *Store,
	transaction *bolt.Tx,
	card *cardapi.Card,
) (cardapi.Card, error) {
	err := storeLoadCardItems(storage, transaction, card)
	if err != nil {
		return cardapi.Card{}, fmt.Errorf(loadCardErrorFormat, err)
	}

	result, err := library.Hydrate(card)
	if err != nil {
		return result, fmt.Errorf(loadCardErrorFormat, err)
	}

	return result, nil
}

func readCardMetadata(bucket *bolt.Bucket, folderID, cardID string) (cardapi.Card, error) {
	data := bucket.Get([]byte(cardID))
	if data == nil {
		return cardapi.Card{}, errCardNotFound
	}

	card, err := decodeCardMetadata(data, folderID, cardID)
	if err != nil {
		return card, fmt.Errorf("read card metadata: %w", err)
	}

	return card, nil
}

func decodeCardMetadata(data []byte, folderID, cardID string) (cardapi.Card, error) {
	var card cardapi.Card

	err := decodeRecord(data, &card)
	if err != nil {
		return cardapi.Card{}, fmt.Errorf("decode card metadata: %w", err)
	}

	if !validCardMetadata(&card, folderID, cardID) {
		return cardapi.Card{}, errInvalidCardMetadata
	}

	return card, nil
}

func validCardMetadata(card *cardapi.Card, folderID, cardID string) bool {
	return card.ID == cardID && card.FolderID == folderID && card.CreatedAt > zero
}

func storeLoadCardItems(storage *Store, transaction *bolt.Tx, card *cardapi.Card) error {
	items := transaction.Bucket([]byte(foldersBucket)).
		Bucket([]byte(card.FolderID)).
		Bucket([]byte(itemsBucket)).
		Bucket([]byte(card.ID))
	reader := newCardContentReader(storage, transaction, card)

	err := cardContentReaderLoad(reader, items)
	if err != nil {
		return fmt.Errorf("load card items: %w", err)
	}

	return nil
}

func cardContentReaderLoad(reader *cardContentReader, items *bolt.Bucket) error {
	if items == nil {
		return errMissingCardContent
	}

	reader.card.Items = []contentapi.Item{}

	err := cardContentReaderReadItems(reader, items)
	if err != nil {
		return fmt.Errorf("read card content items: %w", err)
	}

	return nil
}

func newCardContentReader(
	storage *Store,
	transaction *bolt.Tx,
	card *cardapi.Card,
) *cardContentReader {
	return &cardContentReader{
		settings:  storage.settings,
		card:      card,
		scheduler: storage.scheduler,
		schema:    readSchemaVersion(transaction),
	}
}

func cardContentReaderReadItems(reader *cardContentReader, items *bolt.Bucket) error {
	err := items.ForEach(func(id, data []byte) error { return cardContentReaderAppendItem(reader, id, data) })
	if err != nil {
		return fmt.Errorf("read card content: %w", err)
	}

	return nil
}

func cardContentReaderAppendItem(reader *cardContentReader, id, data []byte) error {
	item, err := decodeContentItem(reader.settings, data)
	if err != nil {
		return fmt.Errorf("decode card content: %w", err)
	}

	if !contentBelongsTo(reader.card, &item, string(id)) {
		return errInvalidContentOwnership
	}

	err = cardContentReaderValidateSchedule(reader, &item)
	if err != nil {
		return fmt.Errorf("validate card content: %w", err)
	}

	reader.card.Items = append(contentapi.Items(reader.card.Items), item)

	return nil
}

func decodeContentItem(settings *Settings, data []byte) (contentapi.Item, error) {
	var item contentapi.Item

	err := decodeRecord(data, &item)
	if err != nil {
		return item, fmt.Errorf("decode content item: %w", err)
	}

	err = library.ValidateItem(&settings.Rules, &item)
	if err != nil {
		return item, fmt.Errorf("validate content item: %w", err)
	}

	return item, nil
}

func cardContentReaderValidateSchedule(reader *cardContentReader, item *contentapi.Item) error {
	if reader.schema != schemaV4 && reader.schema != schemaV5 {
		return nil
	}

	if missingItemSchedule(item) {
		return errMissingItemSchedule
	}

	err := reader.scheduler.Validate(new(contentapi.Schedule(item.Schedule)))
	if err != nil {
		return fmt.Errorf("invalid item schedule: %w", err)
	}

	return nil
}

func missingItemSchedule(item *contentapi.Item) bool {
	return contentapi.Schedule(item.Schedule).Due.IsZero() ||
		contentapi.Schedule(item.Schedule).LastReview.IsZero() ||
		len(contentapi.Reviews(item.Reviews)) == zero
}

func putCard(bucket *bolt.Bucket, card *cardapi.Card) error {
	metadata := *card

	metadata.Items = nil

	return errors.Join(writeRecord(bucket, []byte(card.ID), &metadata))
}

func writeCardBuckets(metadata, items *bolt.Bucket, card *cardapi.Card) error {
	err := putItemsInRoot(items, card)
	if err != nil {
		return fmt.Errorf("write migrated content: %w", err)
	}

	err = putCard(metadata, card)
	if err != nil {
		return fmt.Errorf("write migrated metadata: %w", err)
	}

	return nil
}

func putItems(transaction *bolt.Tx, card *cardapi.Card) error {
	root := transaction.Bucket([]byte(foldersBucket)).
		Bucket([]byte(card.FolderID)).
		Bucket([]byte(itemsBucket))

	err := putItemsInRoot(root, card)
	if err != nil {
		return fmt.Errorf("write content root: %w", err)
	}

	return nil
}

func putItemsInRoot(root *bolt.Bucket, card *cardapi.Card) error {
	bucket, err := replaceItemBucket(root, card.ID)
	if err != nil {
		return fmt.Errorf(putItemsErrorFormat, err)
	}

	for index := range contentapi.Items(card.Items) {
		err = writeContentItem(bucket, &contentapi.Items(card.Items)[index])
		if err != nil {
			return fmt.Errorf(putItemsErrorFormat, err)
		}
	}

	return nil
}

func replaceItemBucket(root *bolt.Bucket, cardID string) (*bolt.Bucket, error) {
	err := deleteExistingItemBucket(root, cardID)
	if err != nil {
		return nil, fmt.Errorf("replace item bucket: %w", err)
	}

	bucket, err := root.CreateBucket([]byte(cardID))
	if err != nil {
		return nil, fmt.Errorf("create item bucket: %w", err)
	}

	return bucket, nil
}

func deleteExistingItemBucket(root *bolt.Bucket, cardID string) error {
	if root.Bucket([]byte(cardID)) == nil {
		return nil
	}

	err := root.DeleteBucket([]byte(cardID))
	if err != nil {
		return fmt.Errorf("delete old item bucket: %w", err)
	}

	return nil
}

func writeContentItem(bucket *bolt.Bucket, item *contentapi.Item) error {
	return errors.Join(writeRecord(bucket, []byte(item.ID), item))
}

func writeRecord(
	bucket *bolt.Bucket,
	key []byte,
	value any,
) error {
	data, err := encodeRecord(value)
	if err != nil {
		return fmt.Errorf("encode database record: %w", err)
	}

	err = bucket.Put(key, data)
	if err != nil {
		return fmt.Errorf("write database record: %w", err)
	}

	return nil
}

// ListCards returns hydrated cards from every folder.
func storeListCards(storage *Store) ([]cardapi.Card, error) {
	reader := cardListing{
		storage: storage, cards: []cardapi.Card{}, root: nil, items: nil, schema: "", folderID: "",
	}

	err := storage.db.View(func(transaction *bolt.Tx) error { return cardListingRead(&reader, transaction) })
	if err != nil {
		return nil, fmt.Errorf("list cards: %w", err)
	}

	return reader.cards, nil
}

func cardListingRead(reader *cardListing, transaction *bolt.Tx) error {
	reader.root = transaction.Bucket([]byte(foldersBucket))
	reader.schema = readSchemaVersion(transaction)

	if reader.root == nil {
		return errMissingFoldersBucket
	}

	err := reader.root.ForEach(func(folderID, value []byte) error { return cardListingAppendFolder(reader, folderID, value) })
	if err != nil {
		return fmt.Errorf("read folders containing cards: %w", err)
	}

	return nil
}

func cardListingAppendFolder(reader *cardListing, folderID, value []byte) error {
	folder := reader.root.Bucket(folderID)
	if !validFolderContent(folder) || value != nil {
		return errInvalidFolderBucket
	}

	reader.folderID = string(folderID)
	reader.items = folder.Bucket([]byte(itemsBucket))

	err := folder.Bucket([]byte(cardsBucket)).ForEach(func(id, data []byte) error { return cardListingAppendCard(reader, id, data) })
	if err != nil {
		return fmt.Errorf("read folder cards: %w", err)
	}

	return nil
}

func cardListingAppendCard(reader *cardListing, id, data []byte) error {
	card, err := cardListingLoadCard(reader, id, data)
	if err != nil {
		return fmt.Errorf("read card %q: %w", id, err)
	}

	reader.cards = append(reader.cards, card)

	return nil
}

func cardListingLoadCard(reader *cardListing, id, data []byte) (cardapi.Card, error) {
	card, err := decodeCardMetadata(data, reader.folderID, string(id))
	if err != nil {
		return card, fmt.Errorf("decode listed metadata: %w", err)
	}

	content := cardContentReader{
		settings: reader.storage.settings, scheduler: reader.storage.scheduler,
		card: &card, schema: reader.schema,
	}

	card, err = cardContentReaderHydrate(&content, reader.items.Bucket(id))
	if err != nil {
		return cardapi.Card{}, fmt.Errorf("load listed content: %w", err)
	}

	return card, nil
}

func cardContentReaderHydrate(reader *cardContentReader, items *bolt.Bucket) (cardapi.Card, error) {
	err := cardContentReaderLoad(reader, items)
	if err != nil {
		return cardapi.Card{}, fmt.Errorf("read content to hydrate: %w", err)
	}

	card, err := library.Hydrate(reader.card)
	if err != nil {
		return cardapi.Card{}, fmt.Errorf("hydrate listed card: %w", err)
	}

	return card, nil
}

// CreateCard persists new content and initializes its schedules.
func storeCreateCard(storage *Store, input *cardapi.CreateInput) (cardapi.Card, error) {
	result5, forwardErr7 := storeSaveCard(storage, input.FolderID, "", contentapi.Drafts(input.Items))
	if forwardErr7 != nil {
		return result5, fmt.Errorf("create card: %w", forwardErr7)
	}

	return result5, nil
}

// UpdateCard updates owned content while preserving current review state.
func storeUpdateCard(storage *Store, input *cardapi.UpdateInput) (cardapi.Card, error) {
	if input.CardID == "" {
		return cardapi.Card{}, errCardIDIsRequired
	}

	result6, forwardErr8 := storeSaveCard(storage,
		input.FolderID,
		input.CardID,
		contentapi.Drafts(input.Items),
	)
	if forwardErr8 != nil {
		return result6, fmt.Errorf("update card: %w", forwardErr8)
	}

	return result6, nil
}

func storeSaveCard(storage *Store,
	folderID, cardID string,
	drafts []contentapi.Draft,
) (cardapi.Card, error) {
	storage.mediaMu.Lock()
	defer storage.mediaMu.Unlock()

	write := newCardWrite(storage, folderID, cardID)
	defer cardWriteDiscardImports(write)

	err := cardWriteExecute(write, drafts)
	if err != nil {
		return cardapi.Card{}, fmt.Errorf(saveCardErrorFormat, err)
	}

	return write.card, nil
}

func newCardWrite(storage *Store, folderID, cardID string) *cardWrite {
	var write cardWrite

	write.storage = storage
	write.folderID = folderID
	write.cardID = cardID

	return &write
}

func cardWriteExecute(write *cardWrite, drafts []contentapi.Draft) error {
	err := cardWritePrepare(write, drafts)
	if err != nil {
		return fmt.Errorf("prepare card edit: %w", err)
	}

	err = write.storage.db.Update(func(transaction *bolt.Tx) error { return cardWritePersist(write, transaction) })
	if err != nil {
		return fmt.Errorf("commit card edit: %w", err)
	}

	write.committed = true
	cardWriteRemoveObsoleteMedia(write)

	return nil
}

func cardWritePrepare(write *cardWrite, drafts []contentapi.Draft) error {
	err := write.storage.db.View(func(transaction *bolt.Tx) error { return cardWriteLoadPrevious(write, transaction) })
	if err != nil {
		return fmt.Errorf("read previous card: %w", err)
	}

	write.items, write.imported, err = library.PrepareItems(&write.storage.settings.Rules,
		context.Background(),
		&library.PreparationInput{Drafts: drafts, Previous: contentapi.Items(write.previous.Items)},
		&library.PreparationDependencies{
			Batch: write.storage.batch,
			ImportMedia: func(
				item *contentapi.Item,
				source string,
			) (contentapi.Item, string, error) {
				return storeImportMedia(write.storage, item, source)
			},
			NewID: newItemID,
		},
	)
	if err != nil {
		return fmt.Errorf("prepare card content: %w", err)
	}

	return nil
}

func cardWriteLoadPrevious(write *cardWrite, transaction *bolt.Tx) error {
	var err error

	switch write.cardID {
	case "":
		err = validateDestinationFolder(transaction, write.folderID)
	default:
		write.previous, err = storeLoadCard(write.storage, transaction, write.folderID, write.cardID)
	}

	if err != nil {
		return fmt.Errorf("load previous card: %w", err)
	}

	return nil
}

func validateDestinationFolder(transaction *bolt.Tx, folderID string) error {
	root := transaction.Bucket([]byte(foldersBucket))
	if root == nil {
		return errMissingFoldersBucket
	}

	if !validFolderContent(root.Bucket([]byte(folderID))) {
		return errFolderNotFoundOrInvalid
	}

	return nil
}

func cardWriteDiscardImports(write *cardWrite) {
	if write.committed {
		return
	}

	for index := range write.imported {
		storeRemoveMediaFile(write.storage, write.imported[index])
	}
}

func cardWritePersist(write *cardWrite, transaction *bolt.Tx) error {
	bucket, err := cardBucket(transaction, write.folderID)
	if err != nil {
		return fmt.Errorf("open destination folder: %w", err)
	}

	err = cardWriteLoadFresh(write, transaction, bucket)
	if err != nil {
		return fmt.Errorf("read current card: %w", err)
	}

	err = cardWritePersistPreparedCard(write, transaction, bucket)
	if err != nil {
		return fmt.Errorf("persist prepared card: %w", err)
	}

	return nil
}

func cardWritePersistPreparedCard(write *cardWrite, transaction *bolt.Tx, bucket *bolt.Bucket) error {
	err := cardWriteMergeSchedules(write)
	if err != nil {
		return fmt.Errorf("merge current schedules: %w", err)
	}

	err = cardWriteWriteRecords(write, transaction, bucket)
	if err != nil {
		return fmt.Errorf("persist card records: %w", err)
	}

	return nil
}

func cardWriteLoadFresh(write *cardWrite, transaction *bolt.Tx, bucket *bolt.Bucket) error {
	var err error

	switch write.cardID {
	case "":
		err = cardWriteAllocate(write, bucket)
	default:
		// Reload after copying media to preserve concurrent reviews and acknowledgments.
		write.card, err = storeLoadCard(write.storage, transaction, write.folderID, write.cardID)
	}

	if err != nil {
		return fmt.Errorf(loadCardForWriteErrorFormat, err)
	}

	return nil
}

func cardWriteAllocate(write *cardWrite, bucket *bolt.Bucket) error {
	var err error

	write.card, err = allocateCard(bucket, write.folderID)
	if err != nil {
		return fmt.Errorf(loadCardForWriteErrorFormat, err)
	}

	return nil
}

func allocateCard(bucket *bolt.Bucket, folderID string) (cardapi.Card, error) {
	sequence, err := bucket.NextSequence()
	if err != nil {
		return cardapi.Card{}, fmt.Errorf("allocate card identifier: %w", err)
	}

	if sequence == zero {
		return cardapi.Card{}, errCardIDSequenceExhausted
	}

	nowMilli := time.Now().UnixMilli()
	card := cardapi.Card{
		ID: fmt.Sprintf(recordIDFormat, sequence), FolderID: folderID,
		CreatedAt: nowMilli, DueAt: nowMilli, Items: nil, LastReviewed: zero,
	}

	if bucket.Get([]byte(card.ID)) != nil {
		return cardapi.Card{}, errCardIDAlreadyExists
	}

	return card, nil
}

func cardWriteMergeSchedules(write *cardWrite) error {
	fresh := indexContentItems(contentapi.Items(write.card.Items))

	write.card.Items = append([]contentapi.Item(nil), write.items...)

	initializedAt := time.Now()

	for index := range contentapi.Items(write.card.Items) {
		item := &contentapi.Items(write.card.Items)[index]

		err := storeRestoreItemSchedule(write.storage, item, fresh[item.ID], initializedAt)
		if err != nil {
			return fmt.Errorf("prepare item schedule: %w", err)
		}
	}

	assignContentOwnership(&write.card)

	return nil
}

func indexContentItems(items []contentapi.Item) map[string]*contentapi.Item {
	indexed := make(map[string]*contentapi.Item, len(items))
	for index := range items {
		indexed[items[index].ID] = &items[index]
	}

	return indexed
}

func storeRestoreItemSchedule(storage *Store,
	item, previous *contentapi.Item,
	now time.Time,
) error {
	if previous != nil {
		item.Schedule = contentapi.Schedule(previous.Schedule)
		item.Reviews = contentapi.Reviews(previous.Reviews)
		item.LastNotifiedDue = previous.LastNotifiedDue

		return nil
	}

	err := library.InitializeItem(item, now, storage.scheduler)
	if err != nil {
		return fmt.Errorf("initialize item schedule: %w", err)
	}

	return nil
}

func assignContentOwnership(card *cardapi.Card) {
	for index := range contentapi.Items(card.Items) {
		contentapi.Items(card.Items)[index].FolderID = card.FolderID
		contentapi.Items(card.Items)[index].CardID = card.ID
	}
}

func cardWriteWriteRecords(write *cardWrite, transaction *bolt.Tx, bucket *bolt.Bucket) error {
	err := writeCardRecords(transaction, bucket, &write.card)
	if err != nil {
		return fmt.Errorf("write card: %w", err)
	}

	write.card, err = storeLoadCard(write.storage, transaction, write.folderID, write.card.ID)
	if err != nil {
		return fmt.Errorf("read saved card: %w", err)
	}

	return nil
}

func writeCardRecords(transaction *bolt.Tx, bucket *bolt.Bucket, card *cardapi.Card) error {
	err := putItems(transaction, card)
	if err != nil {
		return fmt.Errorf("write card content: %w", err)
	}

	err = putCard(bucket, card)
	if err != nil {
		return fmt.Errorf("write card metadata: %w", err)
	}

	return nil
}

func cardWriteRemoveObsoleteMedia(write *cardWrite) {
	kept := make(map[string]bool)

	for index := range contentapi.Items(write.card.Items) {
		kept[contentapi.Items(write.card.Items)[index].ID] = true
	}

	for index := range contentapi.Items(write.previous.Items) {
		item := &contentapi.Items(write.previous.Items)[index]
		if item.Kind != write.storage.settings.Rules.TextKind && !kept[item.ID] {
			storeRemoveMediaFile(write.storage, storeMediaPath(write.storage, item.RelativePath))
		}
	}
}

// ReviewItem records a user rating for an owned content item.
func storeReviewItem(storage *Store, input *cardapi.ReviewInput) (cardapi.Card, error) {
	result7, forwardErr9 := storeReviewItemAt(storage, input, time.Time{})
	if forwardErr9 != nil {
		return result7, fmt.Errorf(reviewItemErrorFormat, forwardErr9)
	}

	return result7, nil
}

func storeReviewItemAt(storage *Store,
	input *cardapi.ReviewInput,
	now time.Time,
) (cardapi.Card, error) {
	if input.Rating < one || input.Rating > four {
		return cardapi.Card{}, errInvalidReviewRating
	}

	review := newCardRating(storage, input.FolderID, input.CardID)

	review.itemID, review.rating, review.now = input.ItemID, input.Rating, now

	err := storage.db.Update(func(transaction *bolt.Tx) error { return cardRatingApply(review, transaction) })
	if err != nil {
		return cardapi.Card{}, fmt.Errorf(reviewItemErrorFormat, err)
	}

	return review.card, nil
}

func newCardRating(storage *Store, folderID, cardID string) *cardRating {
	var review cardRating

	review.storage = storage
	review.folderID = folderID
	review.cardID = cardID

	return &review
}

func cardRatingApply(review *cardRating, transaction *bolt.Tx) error {
	item, err := cardRatingReadAndRate(review, transaction)
	if err != nil {
		return fmt.Errorf(reviewItemAtErrorFormat, err)
	}

	err = cardRatingPersistItem(review, transaction, item)
	if err != nil {
		return fmt.Errorf(reviewItemAtErrorFormat, err)
	}

	return errors.Join(cardRatingHydrate(review))
}

func cardRatingHydrate(review *cardRating) error {
	var err error

	review.card, err = library.Hydrate(&review.card)
	if err != nil {
		return fmt.Errorf(reviewItemAtErrorFormat, err)
	}

	return nil
}

func cardRatingPersistItem(review *cardRating, transaction *bolt.Tx, item *contentapi.Item) error {
	reader := newCardContentReader(review.storage, transaction, &review.card)

	err := cardContentReaderValidateSchedule(reader, item)
	if err != nil {
		return fmt.Errorf("validate reviewed schedule: %w", err)
	}

	bucket := transaction.Bucket([]byte(foldersBucket)).Bucket([]byte(review.folderID)).
		Bucket([]byte(itemsBucket)).Bucket([]byte(review.cardID))

	err = writeContentItem(bucket, item)
	if err != nil {
		return fmt.Errorf("persist reviewed item: %w", err)
	}

	return nil
}

func cardRatingReadAndRate(review *cardRating, transaction *bolt.Tx) (*contentapi.Item, error) {
	var err error

	review.card, err = storeLoadCard(review.storage, transaction, review.folderID, review.cardID)
	if err != nil {
		return nil, fmt.Errorf("read card to review: %w", err)
	}

	item, err := cardRatingRateCard(review)
	if err != nil {
		return nil, fmt.Errorf("review owned content: %w", err)
	}

	return item, nil
}

func cardRatingRateCard(review *cardRating) (*contentapi.Item, error) {
	for index := range contentapi.Items(review.card.Items) {
		item := &contentapi.Items(review.card.Items)[index]
		if item.ID != review.itemID {
			continue
		}

		err := cardRatingRateItem(review, item)
		if err != nil {
			return nil, fmt.Errorf("rate item: %w", err)
		}

		return item, nil
	}

	return nil, errContentItemNotFound
}

func cardRatingRateItem(review *cardRating, item *contentapi.Item) error {
	if review.now.IsZero() {
		review.now = time.Now()
	}

	err := library.ApplyReview(&review.storage.settings.Rules,
		item, &reviewapi.Attempt{At: review.now, Rating: review.rating}, review.storage.scheduler,
	)
	if err != nil {
		return fmt.Errorf("advance reviewed item: %w", err)
	}

	return nil
}

func decodeJSON[Value any](data []byte) (value Value, failure error) {
	failure = json.Unmarshal(data, &value)
	if failure != nil {
		return value, fmt.Errorf("decode json value: %w", failure)
	}

	return value, nil
}

func encodeRecord(record any) ([]byte, error) {
	err := normalizeDecoded(record)
	if err != nil {
		return nil, fmt.Errorf("validate stored record: %w", err)
	}

	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode stored record: %w", err)
	}

	return data, nil
}

// decodeRecord accepts the JSON field names written by every supported schema.
func decodeRecord(data []byte, target any) error {
	normalized, err := normalizeRecord(data)
	if err != nil {
		return fmt.Errorf("normalize stored record: %w", err)
	}

	err = json.Unmarshal(normalized, target)
	if err != nil {
		return fmt.Errorf("decode stored record: %w", err)
	}

	err = normalizeDecoded(target)
	if err != nil {
		return fmt.Errorf("validate decoded record: %w", err)
	}

	return nil
}

func normalizeDecoded(target any) (err error) {
	switch record := target.(type) {
	case *contentapi.Item:
		err = contentapi.NormalizeItem(record)
	case *cardapi.Card:
		err = cardapi.Normalize(record)
	case *legacyItem:
		err = contentapi.NormalizeItem(record.Item)
	}

	if err != nil {
		return fmt.Errorf("validate record values: %w", err)
	}

	return nil
}

func normalizeRecord(data json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == zero {
		return nil, errEmptyStoredRecord
	}

	normalized, err := normalizeValue(trimmed)
	if err != nil {
		return normalized, fmt.Errorf(normalizeRecordErrorFormat, err)
	}

	return normalized, nil
}

func normalizeValue(data json.RawMessage) (normalized json.RawMessage, failure error) {
	switch data[zero] {
	case '{':
		normalized, failure = normalizeObject(data)
	case '[':
		normalized, failure = normalizeArray(data)
	default:
		return data, nil
	}

	if failure != nil {
		return normalized, fmt.Errorf("normalize structured value: %w", failure)
	}

	return normalized, nil
}

func normalizeObject(data json.RawMessage) (json.RawMessage, error) {
	fields, err := decodeJSON[map[string]json.RawMessage](data)
	if err != nil {
		return nil, fmt.Errorf("decode stored object: %w", err)
	}

	result, err := normalizeFields(fields)
	if err != nil {
		return nil, fmt.Errorf("normalize stored fields: %w", err)
	}

	encoded, err := encodeRecord(result)

	return encoded, errors.Join(err)
}

func normalizeFields(fields map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	result := make(map[string]json.RawMessage, len(fields))
	for key := range fields {
		err := normalizeField(result, key, fields[key])
		if err != nil {
			return nil, fmt.Errorf("normalize stored field: %w", err)
		}
	}

	return result, nil
}

func normalizeField(fields map[string]json.RawMessage, key string, value json.RawMessage) error {
	canonical := recordKey(key)
	if _, exists := fields[canonical]; exists {
		return errAmbiguousStoredField
	}

	normalized, err := normalizeRecord(value)
	if err != nil {
		return fmt.Errorf("normalize field value: %w", err)
	}

	fields[canonical] = normalized

	return nil
}

func normalizeArray(data json.RawMessage) (json.RawMessage, error) {
	values, err := decodeJSON[[]json.RawMessage](data)
	if err != nil {
		return nil, fmt.Errorf("decode stored array: %w", err)
	}

	err = normalizeElements(values)
	if err != nil {
		return nil, fmt.Errorf("normalize stored array: %w", err)
	}

	encoded, err := encodeRecord(values)

	return encoded, errors.Join(err)
}

func normalizeElements(values []json.RawMessage) error {
	for index := range values {
		normalized, err := normalizeRecord(values[index])
		if err != nil {
			return fmt.Errorf("normalize stored array item: %w", err)
		}

		values[index] = normalized
	}

	return nil
}

func recordKey(key string) string {
	switch key {
	case legacySourcePath,
		legacyOriginalName,
		legacyPreviewURL,
		legacyByteSize,
		legacyMimeType,
		legacyRelativePath:
		return legacyMediaKey(key)
	case legacyFolderID,
		legacyCardID,
		legacyCreatedAt,
		legacyDueAt,
		legacyLastReviewed,
		legacyLastNotifiedDue:
		return legacyCardKey(key)
	case legacyLastReview, legacyScheduledDays, legacyRemainingSteps:
		return legacyScheduleKey(key)
	default:
		return lowercaseLegacyKey(key)
	}
}

func legacyMediaKey(key string) string {
	return map[string]string{
		legacySourcePath:   "source_path",
		legacyOriginalName: "original_name",
		legacyPreviewURL:   "preview_url",
		legacyByteSize:     "byte_size",
		legacyMimeType:     "mime_type",
		legacyRelativePath: "relative_path",
	}[key]
}

func legacyCardKey(key string) string {
	return map[string]string{
		legacyFolderID:        "folder_id",
		legacyCardID:          "card_id",
		legacyCreatedAt:       "created_at",
		legacyDueAt:           "due_at",
		legacyLastReviewed:    "last_reviewed",
		legacyLastNotifiedDue: "last_notified_due",
	}[key]
}

func legacyScheduleKey(key string) string {
	return map[string]string{
		legacyLastReview:     "last_review",
		legacyScheduledDays:  "scheduled_days",
		legacyRemainingSteps: "remaining_steps",
	}[key]
}

func lowercaseLegacyKey(key string) string {
	switch key {
	case legacyDue,
		legacyStability,
		legacyDifficulty,
		legacyReps,
		legacyLapses,
		legacyState,
		legacyReview,
		legacyRating:
		return strings.ToLower(key)
	default:
		return key
	}
}

func settingsLegacyCollector(settings *Settings,
	card *cardapi.Card,
	old *[]*legacyItem,
) func([]byte, []byte) error {
	return func(id, data []byte) error {
		item, err := settingsReadLegacyItem(settings, id, data, card)
		if err != nil {
			return fmt.Errorf(mergeLegacyItemsErrorFormat, err)
		}

		*old = append(*old, item)

		return nil
	}
}
