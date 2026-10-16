// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package boltstore

import (
	"bytes"
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
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	"raven/internal/library"
	"raven/internal/library/adapters/filesystem"
)

// ClaimAutostartInitialization records the attempt before touching OS settings.
// A missing bucket is normal for existing databases; no schema migration is needed.
func (storage *Store) ClaimAutostartInitialization() (bool, error) {
	claimed := false

	err := storage.db.Update(func(transaction *bolt.Tx) error {
		settings, err := transaction.CreateBucketIfNotExists([]byte(settingsBucket))
		if err != nil {
			return fmt.Errorf(claimAutostartInitializationErrorFormat, err)
		}

		if settings.Get([]byte(autostartInitializedKey)) != nil {
			return nil
		}

		putErr := settings.Put([]byte(autostartInitializedKey), []byte("1"))
		if putErr != nil {
			return fmt.Errorf(claimAutostartInitializationErrorFormat, putErr)
		}

		claimed = true

		return nil
	})
	if err != nil {
		return false, fmt.Errorf("record login startup initialization: %w", err)
	}

	return claimed, nil
}

// MarkNotified only acknowledges schedules that have not changed since delivery.
func (storage *Store) MarkNotified(occurrences []library.DueOccurrence) error {
	forwardErr10 := storage.db.Update(func(transaction *bolt.Tx) error {
		for occurrenceIndex := range occurrences {
			occurrence := &occurrences[occurrenceIndex]

			card, err := storage.loadCard(transaction, occurrence.FolderID, occurrence.CardID)
			if err != nil {
				return fmt.Errorf(markNotifiedErrorFormat, err)
			}

			for i := range card.Items {
				item := &card.Items[i]
				if item.ID == occurrence.ItemID &&
					item.Schedule.Due.UnixMilli() == occurrence.Due {

					item.LastNotifiedDue = occurrence.Due
				}
			}

			putItemsErr := putItems(transaction, &card)
			if putItemsErr != nil {
				return fmt.Errorf(markNotifiedErrorFormat, putItemsErr)
			}
		}

		return nil
	})
	if forwardErr10 != nil {
		return fmt.Errorf(markNotifiedErrorFormat, forwardErr10)
	}

	return nil
}

// ClaimNotificationPrompt persists the one-time authorization prompt decision.
func (storage *Store) ClaimNotificationPrompt() (bool, error) {
	claimed := false

	err := storage.db.Update(func(transaction *bolt.Tx) error {
		meta := transaction.Bucket([]byte(metaBucket))
		if meta == nil {
			return errMissingMetadata
		}

		key := []byte("notificationAuthorizationRequested")
		if meta.Get(key) != nil {
			return nil
		}

		claimed = true

		return meta.Put(key, []byte("true"))
	})
	if err != nil {
		return claimed, fmt.Errorf("claim notification prompt: %w", err)
	}

	return claimed, nil
}

func (storage *Store) initialize(path string) error {
	migration, err := storage.planMigration(path)
	if err != nil {
		return fmt.Errorf("prepare database migration: %w", err)
	}

	if migration.state.version == schemaV5 {
		return nil
	}

	migration.now = time.Now()

	err = storage.db.Update(migration.apply)
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	return nil
}

func (storage *Store) planMigration(path string) (*schemaMigration, error) {
	state, err := storage.readSchema()
	if err != nil {
		return nil, fmt.Errorf("inspect database schema: %w", err)
	}

	migration := &schemaMigration{storage: storage, state: state, now: time.Now()}
	if state.version == schemaV5 {
		return migration, nil
	}

	err = migration.backup(path)
	if err != nil {
		return nil, fmt.Errorf("back up database: %w", err)
	}

	return migration, nil
}

func (storage *Store) readSchema() (schemaState, error) {
	state := schemaState{version: schemaV1, existing: false}

	err := storage.db.View(func(transaction *bolt.Tx) error {
		state.version = readSchemaVersion(transaction)
		state.existing = transaction.Bucket([]byte(foldersBucket)) != nil

		return state.validate()
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

func (state *schemaState) validate() error {
	if !slices.Contains([]string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5}, state.version) {
		return fmt.Errorf(errorFormat, errUnsupportedDatabaseSchema, state.version)
	}

	if state.version != schemaV1 && !state.existing {
		return errMissingFoldersBucket
	}

	return nil
}

func (migration *schemaMigration) backup(path string) error {
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

	err = migration.storage.writeBackup(file)
	if err != nil {
		return fmt.Errorf("write migration backup: %w", errors.Join(err, os.Remove(file.Name())))
	}

	err = syncDirectory(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("sync migration backup: %w", err)
	}

	return nil
}

func (storage *Store) writeBackup(file *os.File) error {
	err := storage.db.View(func(transaction *bolt.Tx) error {
		written, failure := transaction.WriteTo(file)
		if failure != nil {
			return fmt.Errorf("copy migration snapshot: %w", failure)
		}

		if written != transaction.Size() {
			return errIncompleteMigrationBackup
		}

		return nil
	})
	if err == nil {
		err = file.Sync()
	}

	return errors.Join(err, file.Close())
}

func (migration *schemaMigration) apply(transaction *bolt.Tx) error {
	metadata, err := transaction.CreateBucketIfNotExists([]byte(metaBucket))
	if err != nil {
		return fmt.Errorf("create migration metadata: %w", err)
	}

	root, err := transaction.CreateBucketIfNotExists([]byte(foldersBucket))
	if err != nil {
		return fmt.Errorf("create migration folder root: %w", err)
	}

	err = migration.migrateFolders(root)
	if err != nil {
		return fmt.Errorf("migrate folder records: %w", err)
	}

	err = metadata.Put([]byte(schemaKey), []byte(schemaV5))
	if err != nil {
		return fmt.Errorf("commit migrated schema version: %w", err)
	}

	return nil
}

func (migration *schemaMigration) migrateFolders(root *bolt.Bucket) error {
	err := root.ForEach(func(folderID, value []byte) error {
		err := migration.migrateFolder(root, folderID, value)
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

func (migration *schemaMigration) migrateFolder(root *bolt.Bucket, folderID, value []byte) error {
	folder := root.Bucket(folderID)

	err := migration.prepareFolder(folder, value)
	if err != nil {
		return fmt.Errorf("prepare legacy folder: %w", err)
	}

	ids, err := legacyCardIDs(folder.Bucket([]byte(cardsBucket)))
	if err != nil {
		return fmt.Errorf("read legacy card identifiers: %w", err)
	}

	err = migration.migrateCards(folder, folderID, ids)
	if err != nil {
		return fmt.Errorf("migrate legacy cards: %w", err)
	}

	return nil
}

func (migration *schemaMigration) prepareFolder(folder *bolt.Bucket, value []byte) error {
	if value != nil || folder == nil {
		return errInvalidLegacyFolder
	}

	if folder.Bucket([]byte(cardsBucket)) == nil {
		return errInvalidLegacyFolder
	}

	if migration.state.version == schemaV2 && folder.Bucket([]byte(itemsBucket)) == nil {
		return errMissingLegacyContentBucket
	}

	content, err := folder.CreateBucketIfNotExists([]byte(itemsBucket))
	if err != nil {
		return fmt.Errorf("create legacy content root: %w", err)
	}

	if content == nil {
		return errMissingLegacyContentBucket
	}

	return nil
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

func (migration *schemaMigration) migrateCards(
	folder *bolt.Bucket,
	folderID []byte,
	ids [][]byte,
) error {
	for index := range ids {
		err := migration.migrateCard(folder, folderID, ids[index])
		if err != nil {
			return fmt.Errorf("migrate legacy card: %w", err)
		}
	}

	return nil
}

func (migration *schemaMigration) migrateCard(folder *bolt.Bucket, folderID, cardID []byte) error {
	card, err := readLegacyCard(folder.Bucket([]byte(cardsBucket)), folderID, cardID)
	if err != nil {
		return fmt.Errorf("read legacy metadata: %w", err)
	}

	err = migration.loadLegacyContent(folder, &card)
	if err != nil {
		return fmt.Errorf("read legacy content: %w", err)
	}

	err = migration.initializeSchedules(&card)
	if err != nil {
		return fmt.Errorf("initialize legacy schedules: %w", err)
	}

	err = migration.persistCard(folder, &card)
	if err != nil {
		return fmt.Errorf("persist migrated card: %w", err)
	}

	return nil
}

func readLegacyCard(bucket *bolt.Bucket, folderID, cardID []byte) (library.Card, error) {
	var card library.Card

	err := decodeRecord(bucket.Get(cardID), &card)
	if err != nil {
		return card, fmt.Errorf("decode legacy card metadata: %w", err)
	}

	if card.ID != string(cardID) || card.FolderID != string(folderID) {
		return library.Card{}, fmt.Errorf(errorFormat, errInvalidLegacyCardOwnership, cardID)
	}

	card.Items = []library.ContentItem{}

	return card, nil
}

func (migration *schemaMigration) loadLegacyContent(folder *bolt.Bucket, card *library.Card) error {
	var err error

	switch migration.state.version {
	case schemaV1:
		card.Items, err = legacyTextItems(folder.Bucket([]byte(cardsBucket)).Get([]byte(card.ID)))
	case schemaV2:
		card.Items, err = mergeLegacyItems(
			folder.Bucket([]byte(itemsBucket)).Bucket([]byte(card.ID)),
			*card,
		)
	case schemaV3, schemaV4:
		err = migration.loadScheduledContent(folder.Tx(), card)
	default:
		return errUnsupportedDatabaseSchema
	}

	if err != nil {
		return fmt.Errorf("load legacy content records: %w", err)
	}

	return nil
}

func (migration *schemaMigration) loadScheduledContent(
	transaction *bolt.Tx,
	card *library.Card,
) error {
	previous, err := migration.storage.loadCard(transaction, card.FolderID, card.ID)
	if err != nil {
		return fmt.Errorf("read scheduled legacy content: %w", err)
	}

	card.Items = previous.Items

	return nil
}

func legacyTextItems(data []byte) ([]library.ContentItem, error) {
	var previous legacyText

	err := decodeRecord(data, &previous)
	if err != nil {
		return nil, fmt.Errorf("decode legacy text: %w", err)
	}

	if strings.TrimSpace(previous.Prompt) == "" || strings.TrimSpace(previous.Answer) == "" {
		return nil, errInvalidLegacyCard
	}

	items, err := createLegacyTextItems([]string{previous.Prompt, previous.Answer})
	if err != nil {
		return nil, fmt.Errorf("create legacy text content: %w", err)
	}

	return items, nil
}

func createLegacyTextItems(texts []string) ([]library.ContentItem, error) {
	items := make([]library.ContentItem, zero, len(texts))
	for position := range texts {
		item, err := legacyTextItem(texts[position], position)
		if err != nil {
			return nil, fmt.Errorf("create legacy text item: %w", err)
		}

		items = append(items, item)
	}

	return items, nil
}

func legacyTextItem(text string, position int) (library.ContentItem, error) {
	var item library.ContentItem

	id, err := newItemID()
	if err != nil {
		return item, fmt.Errorf("create legacy item identifier: %w", err)
	}

	item.ID = id
	item.Kind = textKind
	item.Position = position
	item.Text = text

	return item, nil
}

func (migration *schemaMigration) initializeSchedules(card *library.Card) error {
	if migration.state.version == schemaV4 {
		return nil
	}

	for index := range card.Items {
		err := library.InitializeItem(
			&card.Items[index],
			migration.now,
			migration.storage.scheduler,
		)
		if err != nil {
			return fmt.Errorf("initialize migrated item: %w", err)
		}
	}

	return nil
}

func (migration *schemaMigration) persistCard(folder *bolt.Bucket, card *library.Card) error {
	for index := range card.Items {
		card.Items[index].FolderID = card.FolderID
		card.Items[index].CardID = card.ID
	}

	err := putItems(folder.Tx(), card)
	if err != nil {
		return fmt.Errorf("write migrated content: %w", err)
	}

	err = putCard(folder.Bucket([]byte(cardsBucket)), *card)
	if err != nil {
		return fmt.Errorf("write migrated metadata: %w", err)
	}

	loaded, err := migration.storage.loadCard(folder.Tx(), card.FolderID, card.ID)
	if err != nil {
		return fmt.Errorf("verify migrated card: %w", err)
	}

	if len(loaded.Items) != len(card.Items) {
		return errMissingCardContent
	}

	return nil
}

// Merge prompt items followed by answer items, retaining every item ID and file.
func mergeLegacyItems(bucket *bolt.Bucket, card library.Card) ([]library.ContentItem, error) {
	if bucket == nil {
		return nil, errMissingLegacyCardContent
	}

	old := []legacyItem{}

	err := bucket.ForEach(func(id, data []byte) error {
		var item legacyItem

		err := decodeRecord(data, &item)
		if err != nil {
			return fmt.Errorf(mergeLegacyItemsErrorFormat, err)
		}

		if item.ID != string(id) || item.CardID != card.ID || item.FolderID != card.FolderID ||
			(item.Side != legacyPromptSide && item.Side != legacyAnswerSide) {

			return errInvalidLegacyContentOwnershipOrSide
		}

		err = library.ValidateItem(item.ContentItem)
		if err != nil {
			return fmt.Errorf(mergeLegacyItemsErrorFormat, err)
		}

		old = append(old, item)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf(mergeLegacyItemsErrorFormat, err)
	}

	sort.Slice(old, func(i, j int) bool {
		if old[i].Side != old[j].Side {
			return old[i].Side == legacyPromptSide
		}

		return old[i].Position < old[j].Position
	})

	counts := map[string]int{legacyPromptSide: zero, legacyAnswerSide: zero}
	items := make([]library.ContentItem, zero, len(old))

	for position, item := range old {
		if item.Position != counts[item.Side] {
			return nil, errInvalidLegacyContentOrder
		}

		counts[item.Side]++

		item.Position = position
		items = append(items, item.ContentItem)
	}

	if counts[legacyPromptSide] == zero || counts[legacyAnswerSide] == zero {
		return nil, errMissingLegacyCardContent
	}

	return items, nil
}

func (storage *Store) mediaPath(relative string) string {
	return filepath.Join(storage.mediaDir, filepath.Base(relative))
}

func inspectMedia(file *os.File, kind, source string) (string, error) {
	declared := library.MediaType(strings.ToLower(filepath.Ext(source)))
	if !supportedMediaKind(kind) || !strings.HasPrefix(declared, kind+"/") {
		return "", errChooseASupportedFileForThisMedia
	}

	header, err := readMediaHeader(file)
	if err != nil {
		return "", fmt.Errorf("read selected media header: %w", err)
	}

	if !compatibleMedia(declared, header) {
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

func compatibleMedia(declared string, header []byte) bool {
	detected, _, _ := strings.Cut(http.DetectContentType(header), ";")
	if detected == declared {
		return true
	}

	switch declared {
	case "audio/wav":
		return detected == "audio/wave"
	case "audio/ogg", "video/ogg":
		return detected == "application/ogg"
	case "audio/mp4":
		return detected == "video/mp4"
	default:
		return compatibleSignature(declared, header)
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
		string(header[isoSignatureStart:isoSignatureEnd]) == isoMediaSignature
}

func avifHeader(header []byte) bool {
	if !isoMediaHeader(header) {
		return false
	}

	brand := string(header[isoSignatureEnd:isoHeaderSize])

	return brand == "avif" || brand == "avis"
}

func (storage *Store) importMedia(
	item library.ContentItem,
	source string,
) (library.ContentItem, string, error) {
	file, err := filesystem.OpenSelected(source)
	if err != nil {
		return item, "", fmt.Errorf("open selected file: %w", err)
	}
	defer storage.closeMediaFile(context.Background(), file)

	imported := item

	err = inspectImportedMedia(file, source, &imported)
	if err != nil {
		return item, "", fmt.Errorf(importMediaErrorFormat, err)
	}

	path, err := storage.persistMedia(file, &imported)
	if err != nil {
		return item, "", fmt.Errorf(importMediaErrorFormat, err)
	}

	return imported, path, nil
}

func inspectImportedMedia(file *os.File, source string, item *library.ContentItem) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect selected file: %w", err)
	}

	if !info.Mode().IsRegular() {
		return errSelectARegularMediaFile
	}

	mimeType, err := inspectMedia(file, item.Kind, source)
	if err != nil {
		return fmt.Errorf("inspect media content: %w", err)
	}

	item.RelativePath = "media/" + item.ID + strings.ToLower(filepath.Ext(source))
	item.OriginalName = filepath.Base(source)
	item.MIMEType = mimeType
	item.ByteSize = info.Size()

	return nil
}

func (storage *Store) persistMedia(file *os.File, item *library.ContentItem) (string, error) {
	temp, err := os.CreateTemp(storage.mediaDir, ".import-*")
	if err != nil {
		return "", fmt.Errorf("create media staging file: %w", err)
	}
	defer storage.removeMediaFile(temp.Name())

	size, err := copyMediaFile(temp, file)
	if err != nil {
		return "", fmt.Errorf("stage media file: %w", err)
	}

	if size != item.ByteSize {
		return "", errSelectedFileChangedDuringImportTryAgain
	}

	path, err := storage.commitMedia(temp.Name(), storage.mediaPath(item.RelativePath))
	if err != nil {
		return "", fmt.Errorf("commit imported media: %w", err)
	}

	return path, nil
}

func copyMediaFile(target *os.File, source io.Reader) (int64, error) {
	size, err := io.Copy(target, source)
	if err == nil {
		err = target.Sync()
	}

	err = errors.Join(err, target.Close())
	if err != nil {
		return size, fmt.Errorf("copy and sync media: %w", err)
	}

	return size, nil
}

func (storage *Store) commitMedia(staged, path string) (string, error) {
	err := os.Rename(staged, path)
	if err != nil {
		return "", fmt.Errorf("rename imported media: %w", err)
	}

	err = syncDirectory(storage.mediaDir)
	if err != nil {
		storage.removeMediaFile(path)

		return "", fmt.Errorf("sync media directory: %w", err)
	}

	return path, nil
}

func (storage *Store) cleanupMedia() error {
	// Validate all referenced records before removing anything.
	cards, err := storage.ListCards()
	if err != nil {
		return fmt.Errorf(cleanupMediaErrorFormat, err)
	}

	err = os.MkdirAll(storage.mediaDir, privateDirectoryMode)
	if err != nil {
		return fmt.Errorf(cleanupMediaErrorFormat, err)
	}

	err = storage.removeUnreferencedMedia(referencedMedia(cards))
	if err != nil {
		return fmt.Errorf(cleanupMediaErrorFormat, err)
	}

	return nil
}

func referencedMedia(cards []library.Card) map[string]bool {
	referenced := make(map[string]bool)

	for cardIndex := range cards {
		for itemIndex := range cards[cardIndex].Items {
			item := &cards[cardIndex].Items[itemIndex]
			if item.Kind != textKind {
				referenced[filepath.Base(item.RelativePath)] = true
			}
		}
	}

	return referenced
}

func (storage *Store) removeUnreferencedMedia(referenced map[string]bool) error {
	entries, err := os.ReadDir(storage.mediaDir)
	if err != nil {
		return fmt.Errorf(cleanupMediaErrorFormat, err)
	}

	for index := range entries {
		if removableMedia(entries[index], referenced) {
			err = os.Remove(filepath.Join(storage.mediaDir, entries[index].Name()))
			if err != nil {
				return fmt.Errorf(cleanupMediaErrorFormat, err)
			}
		}
	}

	return nil
}

func removableMedia(entry os.DirEntry, referenced map[string]bool) bool {
	name := entry.Name()
	managed := strings.HasPrefix(name, ".import-") ||
		library.ValidMediaPath("media/"+name, strings.TrimSuffix(name, filepath.Ext(name)))

	return !entry.IsDir() && managed && !referenced[name]
}

// MediaHandler serves only media records owned by the requested card.
// ServeContent implements HEAD and byte ranges without buffering the file.
func (storage *Store) MediaHandler() http.HandlerFunc { return storage.serveMedia }

func (storage *Store) serveMedia(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.WriteHeader(http.StatusMethodNotAllowed)

		return
	}

	parts := strings.Split(strings.TrimPrefix(request.URL.Path, mediaRoutePrefix), "/")
	if !validMediaRoute(request.URL.Path, parts) {
		http.NotFound(writer, request)

		return
	}

	storage.serveOwnedMedia(writer, request, parts)
}

func validMediaRoute(path string, parts []string) bool {
	return strings.HasPrefix(path, mediaRoutePrefix) && len(parts) == 3 &&
		itemIDPattern.MatchString(parts[2])
}

func (storage *Store) serveOwnedMedia(
	writer http.ResponseWriter,
	request *http.Request,
	parts []string,
) {
	file, item, err := storage.openOwnedMedia(parts)
	if err != nil {
		http.NotFound(writer, request)

		return
	}
	defer storage.closeMediaFile(request.Context(), file)

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(writer, request)

		return
	}

	writer.Header().Set("Content-Type", item.MIMEType)
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Cache-Control", "no-store")
	http.ServeContent(writer, request, item.OriginalName, info.ModTime(), file)
}

func (storage *Store) openOwnedMedia(parts []string) (*os.File, library.ContentItem, error) {
	storage.mediaMu.Lock()
	defer storage.mediaMu.Unlock()

	var item library.ContentItem

	err := storage.db.View(func(transaction *bolt.Tx) error {
		var lookupErr error

		item, lookupErr = storage.lookupMedia(transaction, parts)
		if lookupErr != nil {
			return fmt.Errorf("look up owned media: %w", lookupErr)
		}

		return nil
	})
	if err != nil {
		return nil, item, fmt.Errorf("load media: %w", err)
	}

	file, err := os.OpenInRoot(storage.mediaDir, filepath.Base(item.RelativePath))
	if err != nil {
		return nil, item, fmt.Errorf("open saved media: %w", err)
	}

	return file, item, nil
}

func (storage *Store) lookupMedia(
	transaction *bolt.Tx,
	parts []string,
) (library.ContentItem, error) {
	card, err := storage.loadCard(transaction, parts[zero], parts[one])
	if err != nil {
		return library.ContentItem{}, fmt.Errorf("media handler: %w", err)
	}

	for index := range card.Items {
		item := &card.Items[index]
		if item.ID == parts[2] && item.Kind != textKind {
			return *item, nil
		}
	}

	return library.ContentItem{}, errMediaNotFound
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
func Open(path string, scheduler library.Scheduler, loggers ...*slog.Logger) (*Store, error) {
	db, err := openDatabase(path)
	if err != nil {
		return nil, fmt.Errorf("open library persistence %q: %w", path, err)
	}

	storage := newStore(db, scheduler, path)

	storage.logger = storageLogger(loggers)

	err = storage.prepare(path)
	if err != nil {
		return nil, fmt.Errorf("prepare library persistence: %w", errors.Join(err, db.Close()))
	}

	return storage, nil
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

func newStore(db *bolt.DB, scheduler library.Scheduler, path string) *Store {
	return &Store{
		db:        db,
		scheduler: scheduler,
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

func (storage *Store) prepare(path string) error {
	err := storage.initialize(path)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}

	err = storage.cleanupMedia()
	if err != nil {
		return fmt.Errorf("reconcile database media: %w", err)
	}

	return nil
}

func (storage *Store) closeMediaFile(ctx context.Context, file *os.File) {
	err := file.Close()
	if err != nil {
		storage.logger.WarnContext(ctx, "could not close media file", slog.Any(logErrorKey, err))
	}
}

func (storage *Store) removeMediaFile(path string) {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}

	storage.reportCleanupFailure(err)
}

func (storage *Store) reportCleanupFailure(err error) {
	if err != nil {
		storage.logger.WarnContext(
			context.Background(),
			"could not clean up media",
			slog.Any(logErrorKey, err),
		)
	}
}

// Close closes library persistence after current media operations finish.
func (storage *Store) Close() error {
	storage.mediaMu.Lock()
	defer storage.mediaMu.Unlock()

	forwardErr2 := storage.db.Close()
	if forwardErr2 != nil {
		return fmt.Errorf("close: %w", forwardErr2)
	}

	return nil
}

// ListFolders returns folder metadata in creation order.
func (storage *Store) ListFolders() ([]library.Folder, error) {
	folders := []library.Folder{}

	err := storage.db.View(func(transaction *bolt.Tx) error {
		root := transaction.Bucket([]byte(foldersBucket))
		if root == nil {
			return errMissingFoldersBucket
		}

		return root.ForEach(func(key, value []byte) error {
			folder, err := readFolder(root, key, value)
			if err != nil {
				return fmt.Errorf("read folder %q: %w", key, err)
			}

			folders = append(folders, folder)

			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}

	return folders, nil
}

func readFolder(root *bolt.Bucket, key, value []byte) (library.Folder, error) {
	bucket := root.Bucket(key)
	if value != nil || bucket == nil || bucket.Bucket([]byte(cardsBucket)) == nil {
		return library.Folder{}, fmt.Errorf("%w %q", errInvalidFolderBucket, key)
	}

	folder, err := decodeFolder(bucket.Get([]byte(metadataKey)), string(key))
	if err != nil {
		return library.Folder{}, fmt.Errorf("decode folder %q: %w", key, err)
	}

	return folder, nil
}

func decodeFolder(data []byte, id string) (library.Folder, error) {
	var folder library.Folder

	err := decodeRecord(data, &folder)
	if err != nil {
		return folder, fmt.Errorf("decode folder metadata: %w", err)
	}

	if folder.ID != id || strings.TrimSpace(folder.Title) == "" {
		return library.Folder{}, fmt.Errorf("%w %q", errInvalidFolderMetadata, id)
	}

	if folder.Tags == nil {
		folder.Tags = []string{}
	}

	return folder, nil
}

// CreateFolder persists normalized metadata in a new folder.
func (storage *Store) CreateFolder(input library.CreateFolderInput) (library.Folder, error) {
	folder, err := library.NormalizeFolder(input)
	if err != nil {
		return library.Folder{}, fmt.Errorf(createFolderErrorFormat, err)
	}

	err = storage.db.Update(func(transaction *bolt.Tx) error {
		err := persistNewFolder(transaction, &folder)
		if err != nil {
			return fmt.Errorf(createFolderErrorFormat, err)
		}

		return nil
	})
	if err != nil {
		return library.Folder{}, fmt.Errorf(createFolderErrorFormat, err)
	}

	return folder, nil
}

func persistNewFolder(transaction *bolt.Tx, folder *library.Folder) error {
	root := transaction.Bucket([]byte(foldersBucket))
	if root == nil {
		return errMissingFoldersBucket
	}

	id, err := nextFolderID(root)
	if err != nil {
		return fmt.Errorf("allocate folder: %w", err)
	}

	folder.ID = id

	bucket, err := root.CreateBucket([]byte(id))
	if err != nil {
		return fmt.Errorf("create folder bucket: %w", err)
	}

	err = initializeFolder(bucket, folder)
	if err != nil {
		return fmt.Errorf("initialize folder: %w", err)
	}

	return nil
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

func initializeFolder(bucket *bolt.Bucket, folder *library.Folder) error {
	err := writeFolderMetadata(bucket, folder)
	if err != nil {
		return fmt.Errorf("write folder metadata: %w", err)
	}

	for _, name := range []string{cardsBucket, itemsBucket} {
		_, err = bucket.CreateBucket([]byte(name))
		if err != nil {
			return fmt.Errorf("create folder content bucket: %w", err)
		}
	}

	return nil
}

func writeFolderMetadata(bucket *bolt.Bucket, folder *library.Folder) error {
	data, err := json.Marshal(folder)
	if err != nil {
		return fmt.Errorf("encode folder metadata: %w", err)
	}

	err = bucket.Put([]byte(metadataKey), data)
	if err != nil {
		return fmt.Errorf("save folder metadata: %w", err)
	}

	return nil
}

// DeleteCard deletes an owned card and reconciles its media files.
func (storage *Store) DeleteCard(folderID, cardID string) error {
	storage.mediaMu.Lock()
	defer storage.mediaMu.Unlock()

	err := storage.db.Update(func(transaction *bolt.Tx) error {
		_, loadCardErr4 := storage.loadCard(transaction, folderID, cardID)
		if loadCardErr4 != nil {
			return fmt.Errorf(deleteCardErrorFormat, loadCardErr4)
		}

		folder := transaction.Bucket([]byte(foldersBucket)).Bucket([]byte(folderID))

		err := folder.Bucket([]byte(itemsBucket)).DeleteBucket([]byte(cardID))
		if err != nil {
			return fmt.Errorf(deleteCardErrorFormat, err)
		}

		return folder.Bucket([]byte(cardsBucket)).Delete([]byte(cardID))
	})
	if err != nil {
		return fmt.Errorf(deleteCardErrorFormat, err)
	}

	// Any failed file removal is retried by startup cleanup.
	storage.reportCleanupFailure(storage.cleanupMedia())

	return nil
}

// DeleteFolder deletes a folder and reconciles its media files.
func (storage *Store) DeleteFolder(folderID string) error {
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

	storage.reportCleanupFailure(storage.cleanupMedia())

	return nil
}

func newItemID() (string, error) {
	var id [16]byte

	_, readErr := rand.Read(id[:])
	if readErr != nil {
		return "", fmt.Errorf("new item i d: %w", readErr)
	}

	return hex.EncodeToString(id[:]), nil
}

func cardBucket(transaction *bolt.Tx, folderID string) (*bolt.Bucket, error) {
	root := transaction.Bucket([]byte(foldersBucket))
	if root == nil {
		return nil, errMissingFoldersBucket
	}

	folder := root.Bucket([]byte(folderID))
	if folder == nil || folder.Bucket([]byte(cardsBucket)) == nil ||
		folder.Bucket([]byte(itemsBucket)) == nil {
		return nil, errFolderNotFoundOrInvalid
	}

	return folder.Bucket([]byte(cardsBucket)), nil
}

func (storage *Store) loadCard(
	transaction *bolt.Tx,
	folderID, cardID string,
) (library.Card, error) {
	bucket, err := cardBucket(transaction, folderID)
	if err != nil {
		return library.Card{}, fmt.Errorf(loadCardErrorFormat, err)
	}

	card, err := readCardMetadata(bucket, folderID, cardID)
	if err != nil {
		return library.Card{}, fmt.Errorf(loadCardErrorFormat, err)
	}

	err = storage.loadCardItems(transaction, &card)
	if err != nil {
		return library.Card{}, fmt.Errorf(loadCardErrorFormat, err)
	}

	result, err := library.Hydrate(card)
	if err != nil {
		return result, fmt.Errorf(loadCardErrorFormat, err)
	}

	return result, nil
}

func readCardMetadata(bucket *bolt.Bucket, folderID, cardID string) (library.Card, error) {
	data := bucket.Get([]byte(cardID))
	if data == nil {
		return library.Card{}, errCardNotFound
	}

	var card library.Card

	err := decodeRecord(data, &card)
	if err != nil {
		return library.Card{}, fmt.Errorf("decode card metadata: %w", err)
	}

	if card.ID != cardID || card.FolderID != folderID || card.CreatedAt <= zero {
		return library.Card{}, errInvalidCardMetadata
	}

	return card, nil
}

func (storage *Store) loadCardItems(transaction *bolt.Tx, card *library.Card) error {
	items := transaction.Bucket([]byte(foldersBucket)).
		Bucket([]byte(card.FolderID)).
		Bucket([]byte(itemsBucket)).
		Bucket([]byte(card.ID))
	if items == nil {
		return errMissingCardContent
	}

	card.Items = []library.ContentItem{}

	reader := cardContentReader{
		card:      card,
		scheduler: storage.scheduler,
		schema:    readSchemaVersion(transaction),
	}

	err := items.ForEach(reader.appendItem)
	if err != nil {
		return fmt.Errorf("read card content: %w", err)
	}

	return nil
}

func (reader *cardContentReader) appendItem(id, data []byte) error {
	item, err := decodeContentItem(data)
	if err != nil {
		return fmt.Errorf("decode card content: %w", err)
	}

	if item.ID != string(id) || item.CardID != reader.card.ID ||
		item.FolderID != reader.card.FolderID {

		return errInvalidContentOwnership
	}

	err = reader.validateSchedule(&item)
	if err != nil {
		return fmt.Errorf("validate card content: %w", err)
	}

	reader.card.Items = append(reader.card.Items, item)

	return nil
}

func decodeContentItem(data []byte) (library.ContentItem, error) {
	var item library.ContentItem

	err := decodeRecord(data, &item)
	if err != nil {
		return item, fmt.Errorf("decode content item: %w", err)
	}

	err = library.ValidateItem(item)
	if err != nil {
		return item, fmt.Errorf("validate content item: %w", err)
	}

	return item, nil
}

func (reader *cardContentReader) validateSchedule(item *library.ContentItem) error {
	if reader.schema != schemaV4 && reader.schema != schemaV5 {
		return nil
	}

	if missingItemSchedule(item) {
		return errMissingItemSchedule
	}

	err := reader.scheduler.Validate(item.Schedule)
	if err != nil {
		return fmt.Errorf("invalid item schedule: %w", err)
	}

	return nil
}

func missingItemSchedule(item *library.ContentItem) bool {
	return item.Schedule.Due.IsZero() || item.Schedule.LastReview.IsZero() ||
		len(item.Reviews) == zero
}

func putCard(bucket *bolt.Bucket, card library.Card) error {
	card.Items = nil

	data, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf(putCardErrorFormat, err)
	}

	forwardErr6 := bucket.Put([]byte(card.ID), data)
	if forwardErr6 != nil {
		return fmt.Errorf(putCardErrorFormat, forwardErr6)
	}

	return nil
}

func putItems(transaction *bolt.Tx, card *library.Card) error {
	root := transaction.Bucket([]byte(foldersBucket)).
		Bucket([]byte(card.FolderID)).
		Bucket([]byte(itemsBucket))

	bucket, err := replaceItemBucket(root, card.ID)
	if err != nil {
		return fmt.Errorf(putItemsErrorFormat, err)
	}

	for index := range card.Items {
		err = writeContentItem(bucket, &card.Items[index])
		if err != nil {
			return fmt.Errorf(putItemsErrorFormat, err)
		}
	}

	return nil
}

func replaceItemBucket(root *bolt.Bucket, cardID string) (*bolt.Bucket, error) {
	if root.Bucket([]byte(cardID)) != nil {
		err := root.DeleteBucket([]byte(cardID))
		if err != nil {
			return nil, fmt.Errorf("replace item bucket: %w", err)
		}
	}

	bucket, err := root.CreateBucket([]byte(cardID))
	if err != nil {
		return nil, fmt.Errorf("create item bucket: %w", err)
	}

	return bucket, nil
}

func writeContentItem(bucket *bolt.Bucket, item *library.ContentItem) error {
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("encode content item: %w", err)
	}

	err = bucket.Put([]byte(item.ID), data)
	if err != nil {
		return fmt.Errorf("write content item: %w", err)
	}

	return nil
}

// ListCards returns hydrated cards from every folder.
func (storage *Store) ListCards() ([]library.Card, error) {
	cards := []library.Card{}

	err := storage.db.View(func(transaction *bolt.Tx) error {
		root := transaction.Bucket([]byte(foldersBucket))
		if root == nil {
			return errMissingFoldersBucket
		}

		return root.ForEach(func(folderID, value []byte) error {
			bucket, err := cardBucket(transaction, string(folderID))
			if err != nil || value != nil {
				return errInvalidFolderBucket
			}

			return bucket.ForEach(func(id, _ []byte) error {
				card, loadCardErr3 := storage.loadCard(transaction, string(folderID), string(id))
				if loadCardErr3 != nil {
					return fmt.Errorf("read card %q: %w", id, loadCardErr3)
				}

				cards = append(cards, card)

				return nil
			})
		})
	})
	if err != nil {
		return nil, fmt.Errorf("list cards: %w", err)
	}

	return cards, nil
}

// CreateCard persists new content and initializes its schedules.
func (storage *Store) CreateCard(input library.CreateCardInput) (library.Card, error) {
	result5, forwardErr7 := storage.saveCard(input.FolderID, "", input.Items)
	if forwardErr7 != nil {
		return result5, fmt.Errorf("create card: %w", forwardErr7)
	}

	return result5, nil
}

// UpdateCard updates owned content while preserving current review state.
func (storage *Store) UpdateCard(input library.UpdateCardInput) (library.Card, error) {
	if input.CardID == "" {
		return library.Card{}, errCardIDIsRequired
	}

	result6, forwardErr8 := storage.saveCard(input.FolderID, input.CardID, input.Items)
	if forwardErr8 != nil {
		return result6, fmt.Errorf("update card: %w", forwardErr8)
	}

	return result6, nil
}

func (storage *Store) saveCard(
	folderID, cardID string,
	drafts []library.ItemDraft,
) (library.Card, error) {
	storage.mediaMu.Lock()
	defer storage.mediaMu.Unlock()

	write := cardWrite{
		storage: storage, folderID: folderID, cardID: cardID,
		previous: library.Card{}, card: library.Card{}, items: nil, imported: nil, committed: false,
	}
	defer write.discardImports()

	err := write.execute(drafts)
	if err != nil {
		return library.Card{}, fmt.Errorf(saveCardErrorFormat, err)
	}

	return write.card, nil
}

func (write *cardWrite) execute(drafts []library.ItemDraft) error {
	err := write.prepare(drafts)
	if err != nil {
		return fmt.Errorf("prepare card edit: %w", err)
	}

	err = write.storage.db.Update(write.persist)
	if err != nil {
		return fmt.Errorf("commit card edit: %w", err)
	}

	write.committed = true
	write.removeObsoleteMedia()

	return nil
}

func (write *cardWrite) prepare(drafts []library.ItemDraft) error {
	err := write.storage.db.View(write.loadPrevious)
	if err != nil {
		return fmt.Errorf("read previous card: %w", err)
	}

	write.items, write.imported, err = library.PrepareItems(
		drafts, write.previous.Items, write.storage.importMedia, newItemID,
	)
	if err != nil {
		return fmt.Errorf("prepare card content: %w", err)
	}

	return nil
}

func (write *cardWrite) loadPrevious(transaction *bolt.Tx) error {
	if write.cardID == "" {
		_, err := cardBucket(transaction, write.folderID)
		if err != nil {
			return fmt.Errorf("validate destination folder: %w", err)
		}

		return nil
	}

	var err error

	write.previous, err = write.storage.loadCard(transaction, write.folderID, write.cardID)
	if err != nil {
		return fmt.Errorf("load previous card: %w", err)
	}

	return nil
}

func (write *cardWrite) discardImports() {
	if write.committed {
		return
	}

	for index := range write.imported {
		write.storage.removeMediaFile(write.imported[index])
	}
}

func (write *cardWrite) persist(transaction *bolt.Tx) error {
	bucket, err := cardBucket(transaction, write.folderID)
	if err != nil {
		return fmt.Errorf("open destination folder: %w", err)
	}

	err = write.loadFresh(transaction, bucket)
	if err != nil {
		return fmt.Errorf("read current card: %w", err)
	}

	err = write.mergeSchedules()
	if err != nil {
		return fmt.Errorf("merge current schedules: %w", err)
	}

	err = write.writeRecords(transaction, bucket)
	if err != nil {
		return fmt.Errorf("persist card records: %w", err)
	}

	return nil
}

func (write *cardWrite) loadFresh(transaction *bolt.Tx, bucket *bolt.Bucket) error {
	var err error

	if write.cardID == "" {
		write.card, err = allocateCard(bucket, write.folderID)
	} else {
		// Reload after file copying to preserve concurrent reviews and acknowledgments.
		write.card, err = write.storage.loadCard(transaction, write.folderID, write.cardID)
	}

	if err != nil {
		return fmt.Errorf("load card for write: %w", err)
	}

	return nil
}

func allocateCard(bucket *bolt.Bucket, folderID string) (library.Card, error) {
	sequence, err := bucket.NextSequence()
	if err != nil {
		return library.Card{}, fmt.Errorf("allocate card identifier: %w", err)
	}

	if sequence == zero {
		return library.Card{}, errCardIDSequenceExhausted
	}

	nowMilli := time.Now().UnixMilli()
	card := library.Card{
		ID: fmt.Sprintf(recordIDFormat, sequence), FolderID: folderID,
		CreatedAt: nowMilli, DueAt: nowMilli, Items: nil, LastReviewed: zero,
	}

	if bucket.Get([]byte(card.ID)) != nil {
		return library.Card{}, errCardIDAlreadyExists
	}

	return card, nil
}

func (write *cardWrite) mergeSchedules() error {
	fresh := make(map[string]*library.ContentItem)

	for index := range write.card.Items {
		item := &write.card.Items[index]

		fresh[item.ID] = item
	}

	write.card.Items = append([]library.ContentItem(nil), write.items...)

	initializedAt := time.Now()

	for index := range write.card.Items {
		item := &write.card.Items[index]

		err := write.storage.restoreItemSchedule(item, fresh[item.ID], initializedAt)
		if err != nil {
			return fmt.Errorf("prepare item schedule: %w", err)
		}
	}

	assignContentOwnership(&write.card)

	return nil
}

func (storage *Store) restoreItemSchedule(
	item, previous *library.ContentItem,
	now time.Time,
) error {
	if previous != nil {
		item.Schedule = previous.Schedule
		item.Reviews = previous.Reviews
		item.LastNotifiedDue = previous.LastNotifiedDue

		return nil
	}

	err := library.InitializeItem(item, now, storage.scheduler)
	if err != nil {
		return fmt.Errorf("initialize item schedule: %w", err)
	}

	return nil
}

func assignContentOwnership(card *library.Card) {
	for index := range card.Items {
		card.Items[index].FolderID = card.FolderID
		card.Items[index].CardID = card.ID
	}
}

func (write *cardWrite) writeRecords(transaction *bolt.Tx, bucket *bolt.Bucket) error {
	err := writeCardRecords(transaction, bucket, &write.card)
	if err != nil {
		return fmt.Errorf("write card: %w", err)
	}

	write.card, err = write.storage.loadCard(transaction, write.folderID, write.card.ID)
	if err != nil {
		return fmt.Errorf("read saved card: %w", err)
	}

	return nil
}

func writeCardRecords(transaction *bolt.Tx, bucket *bolt.Bucket, card *library.Card) error {
	err := putItems(transaction, card)
	if err != nil {
		return fmt.Errorf("write card content: %w", err)
	}

	err = putCard(bucket, *card)
	if err != nil {
		return fmt.Errorf("write card metadata: %w", err)
	}

	return nil
}

func (write *cardWrite) removeObsoleteMedia() {
	kept := make(map[string]bool)

	for index := range write.card.Items {
		kept[write.card.Items[index].ID] = true
	}

	for index := range write.previous.Items {
		item := &write.previous.Items[index]
		if item.Kind != textKind && !kept[item.ID] {
			write.storage.removeMediaFile(write.storage.mediaPath(item.RelativePath))
		}
	}
}

// ReviewItem records a user rating for an owned content item.
func (storage *Store) ReviewItem(
	folderID, cardID, itemID string,
	rating int,
) (library.Card, error) {
	result7, forwardErr9 := storage.reviewItemAt(folderID, cardID, itemID, rating, time.Time{})
	if forwardErr9 != nil {
		return result7, fmt.Errorf(reviewItemErrorFormat, forwardErr9)
	}

	return result7, nil
}

func (storage *Store) reviewItemAt(
	folderID, cardID, itemID string,
	rating int,
	now time.Time,
) (library.Card, error) {
	if rating < one || rating > four {
		return library.Card{}, errInvalidReviewRating
	}

	var card library.Card

	err := storage.db.Update(func(transaction *bolt.Tx) error {
		var err error

		card, err = storage.loadCard(transaction, folderID, cardID)
		if err != nil {
			return fmt.Errorf(reviewItemAtErrorFormat, err)
		}

		found := false

		for i := range card.Items {
			item := &card.Items[i]
			if item.ID != itemID {
				continue
			}

			if now.IsZero() {
				now = time.Now()
			}

			applyReviewErr := library.ApplyReview(item, now, rating, storage.scheduler)
			if applyReviewErr != nil {
				return fmt.Errorf(reviewItemAtErrorFormat, applyReviewErr)
			}

			found = true
		}

		if !found {
			return errContentItemNotFound
		}

		putItemsErr3 := putItems(transaction, &card)
		if putItemsErr3 != nil {
			return fmt.Errorf(reviewItemAtErrorFormat, putItemsErr3)
		}

		card, err = storage.loadCard(transaction, folderID, cardID)
		if err != nil {
			return fmt.Errorf(reviewItemAtErrorFormat, err)
		}

		return nil
	})
	if err != nil {
		return library.Card{}, fmt.Errorf(reviewItemErrorFormat, err)
	}

	return card, nil
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

	return nil
}

func normalizeRecord(data json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == zero {
		return nil, errEmptyStoredRecord
	}

	switch trimmed[zero] {
	case '{':
		result4, forwardErr5 := normalizeObject(trimmed)
		if forwardErr5 != nil {
			return result4, fmt.Errorf(normalizeRecordErrorFormat, forwardErr5)
		}

		return result4, nil
	case '[':
		result3, forwardErr4 := normalizeArray(trimmed)
		if forwardErr4 != nil {
			return result3, fmt.Errorf(normalizeRecordErrorFormat, forwardErr4)
		}

		return result3, nil
	default:
		return trimmed, nil
	}
}

func normalizeObject(data json.RawMessage) (json.RawMessage, error) {
	var fields map[string]json.RawMessage

	err := json.Unmarshal(data, &fields)
	if err != nil {
		return nil, fmt.Errorf("decode stored object: %w", err)
	}

	result := make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		err = normalizeField(result, key, value)
		if err != nil {
			return nil, fmt.Errorf("normalize stored field: %w", err)
		}
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode stored object: %w", err)
	}

	return encoded, nil
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
	var values []json.RawMessage

	err := json.Unmarshal(data, &values)
	if err != nil {
		return nil, fmt.Errorf("decode stored array: %w", err)
	}

	for index := range values {
		values[index], err = normalizeRecord(values[index])
		if err != nil {
			return nil, fmt.Errorf("normalize stored array item: %w", err)
		}
	}

	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("encode stored array: %w", err)
	}

	return encoded, nil
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
