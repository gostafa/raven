// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package boltstore

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"

	"raven/internal/library"
)

// ClaimAutostartInitialization records the attempt before touching OS settings.
// A missing bucket is normal for existing databases; no schema migration is needed.
func (storage *Store) ClaimAutostartInitialization() (bool, error) {
	claimed := false

	err := storage.db.Update(func(transaction *bolt.Tx) error {
		settings, err := transaction.CreateBucketIfNotExists([]byte(settingsBucket))
		if err != nil {
			return fmt.Errorf("claim autostart initialization: %w", err)
		}

		if settings.Get([]byte(autostartInitializedKey)) != nil {
			return nil
		}

		putErr := settings.Put([]byte(autostartInitializedKey), []byte("1"))
		if putErr != nil {
			return fmt.Errorf("claim autostart initialization: %w", putErr)
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
		for _, occurrence := range occurrences {
			card, err := storage.loadCard(transaction, occurrence.FolderID, occurrence.CardID)
			if err != nil {
				return fmt.Errorf("mark notified: %w", err)
			}

			for i := range card.Items {
				item := &card.Items[i]
				if item.ID == occurrence.ItemID && item.Schedule.Due.UnixMilli() == occurrence.Due {
					item.LastNotifiedDue = occurrence.Due
				}
			}

			putItemsErr := putItems(transaction, card)
			if putItemsErr != nil {
				return fmt.Errorf("mark notified: %w", putItemsErr)
			}
		}

		return nil
	})
	if forwardErr10 != nil {
		return fmt.Errorf("mark notified: %w", forwardErr10)
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

	if migration == nil {
		return nil
	}

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

	if state.version == schemaV5 {
		return nil, nil
	}

	migration := &schemaMigration{storage: storage, state: state, now: time.Now()}

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
	if len(version) == 0 {
		return schemaV1
	}

	return string(version)
}

func (state *schemaState) validate() error {
	if !slices.Contains([]string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5}, state.version) {
		return fmt.Errorf("%w %q", errUnsupportedDatabaseSchema, state.version)
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
		return library.Card{}, fmt.Errorf("%w %q", errInvalidLegacyCardOwnership, cardID)
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

	return createLegacyTextItems([]string{previous.Prompt, previous.Answer})
}

func createLegacyTextItems(texts []string) ([]library.ContentItem, error) {
	items := make([]library.ContentItem, 0, len(texts))
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

	err := putItems(folder.Tx(), *card)
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
			return fmt.Errorf("merge legacy items: %w", err)
		}

		if item.ID != string(id) || item.CardID != card.ID || item.FolderID != card.FolderID ||
			(item.Side != legacyPromptSide && item.Side != legacyAnswerSide) {
			return errInvalidLegacyContentOwnershipOrSide
		}

		err = library.ValidateItem(item.ContentItem)
		if err != nil {
			return fmt.Errorf("merge legacy items: %w", err)
		}

		old = append(old, item)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("merge legacy items: %w", err)
	}

	sort.Slice(old, func(i, j int) bool {
		if old[i].Side != old[j].Side {
			return old[i].Side == legacyPromptSide
		}

		return old[i].Position < old[j].Position
	})

	counts := map[string]int{legacyPromptSide: 0, legacyAnswerSide: 0}
	items := make([]library.ContentItem, 0, len(old))

	for position, item := range old {
		if item.Position != counts[item.Side] {
			return nil, errInvalidLegacyContentOrder
		}

		counts[item.Side]++

		item.Position = position
		items = append(items, item.ContentItem)
	}

	if counts[legacyPromptSide] == 0 || counts[legacyAnswerSide] == 0 {
		return nil, errMissingLegacyCardContent
	}

	return items, nil
}

func (storage *Store) mediaPath(relative string) string {
	return filepath.Join(storage.mediaDir, filepath.Base(relative))
}

func inspectMedia(file *os.File, kind, source string) (string, error) {
	declared := library.MediaType(strings.ToLower(filepath.Ext(source)))
	if (kind != "audio" && kind != "image" && kind != "video") ||
		!strings.HasPrefix(declared, kind+"/") {
		return "", errChooseASupportedFileForThisMedia
	}

	var header [512]byte

	notifier, err := file.Read(header[:])

	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("inspect media: %w", err)
	}

	if notifier == 0 {
		return "", errEmptyMediaFile
	}

	detected, _, _ := strings.Cut(http.DetectContentType(header[:notifier]), ";")
	// Ogg, WebM, and MP4 containers can carry audio or video. AVIF/FLAC
	// may be reported as octet-stream by Go's sniffer; check their signatures.
	compatible := detected == declared ||
		(declared == "audio/wav" && detected == "audio/wave") ||
		((declared == "audio/ogg" || declared == "video/ogg") && detected == "application/ogg") ||
		(declared == "audio/mp4" && detected == "video/mp4") ||
		(declared == "video/quicktime" && notifier >= 12 && string(header[4:8]) == isoMediaSignature) ||
		(declared == "image/avif" && notifier >= 12 && string(header[4:8]) == isoMediaSignature && (string(header[8:12]) == "avif" || string(header[8:12]) == "avis")) ||
		(declared == "audio/flac" && notifier >= 4 && string(header[:4]) == "fLaC")
	if !compatible {
		return "", fmt.Errorf("file contents do not match %s", declared)
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return declared, fmt.Errorf("inspect media: %w", err)
	}

	return declared, nil
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}

	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	defer dir.Close()

	forwardErr := dir.Sync()
	if forwardErr != nil {
		return fmt.Errorf("sync directory: %w", forwardErr)
	}

	return nil
}

func (storage *Store) importMedia(
	item library.ContentItem,
	source string,
) (library.ContentItem, string, error) {
	file, err := os.Open(source)
	if err != nil {
		return item, "", fmt.Errorf("open selected file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return item, "", fmt.Errorf("import media: %w", err)
	}

	if !info.Mode().IsRegular() {
		return item, "", errSelectARegularMediaFile
	}

	mimeType, err := inspectMedia(file, item.Kind, source)
	if err != nil {
		return item, "", fmt.Errorf("import media: %w", err)
	}

	temp, err := os.CreateTemp(storage.mediaDir, ".import-*")
	if err != nil {
		return item, "", fmt.Errorf("import media: %w", err)
	}
	defer os.Remove(temp.Name())

	size, copyErr := io.Copy(temp, file)
	if copyErr == nil {
		copyErr = temp.Sync()
	}

	closeErr := temp.Close()

	if copyErr != nil {
		return item, "", fmt.Errorf("import media: %w", copyErr)
	}

	if closeErr != nil {
		return item, "", fmt.Errorf("import media: %w", closeErr)
	}

	if size != info.Size() {
		return item, "", errSelectedFileChangedDuringImportTryAgain
	}

	item.RelativePath = "media/" + item.ID + strings.ToLower(filepath.Ext(source))

	path := storage.mediaPath(item.RelativePath)

	renameErr := os.Rename(temp.Name(), path)
	if renameErr != nil {
		return item, "", fmt.Errorf("import media: %w", renameErr)
	}

	syncDirectoryErr2 := syncDirectory(storage.mediaDir)
	if syncDirectoryErr2 != nil {
		_ = os.Remove(path)

		return item, "", fmt.Errorf("import media: %w", syncDirectoryErr2)
	}

	item.OriginalName = filepath.Base(source)
	item.MIMEType = mimeType
	item.ByteSize = size

	return item, path, nil
}

func (storage *Store) cleanupMedia() error {
	// Validate all referenced records before removing anything.
	cards, err := storage.ListCards()
	if err != nil {
		return fmt.Errorf("cleanup media: %w", err)
	}

	mkdirAllErr := os.MkdirAll(storage.mediaDir, privateDirectoryMode)
	if mkdirAllErr != nil {
		return fmt.Errorf("cleanup media: %w", mkdirAllErr)
	}

	referenced := make(map[string]bool)

	for _, card := range cards {
		for _, item := range card.Items {
			if item.Kind != textKind {
				referenced[filepath.Base(item.RelativePath)] = true
			}
		}
	}

	entries, err := os.ReadDir(storage.mediaDir)
	if err != nil {
		return fmt.Errorf("cleanup media: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		managed := strings.HasPrefix(name, ".import-") ||
			library.ValidMediaPath("media/"+name, strings.TrimSuffix(name, filepath.Ext(name)))

		if !entry.IsDir() && managed && !referenced[name] {
			removeErr := os.Remove(filepath.Join(storage.mediaDir, name))
			if removeErr != nil {
				return fmt.Errorf("cleanup media: %w", removeErr)
			}
		}
	}

	return nil
}

// MediaHandler serves only media records owned by the requested card.
// ServeContent implements HEAD and byte ranges without buffering the file.
func (storage *Store) MediaHandler() http.HandlerFunc {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.WriteHeader(http.StatusMethodNotAllowed)

			return
		}

		parts := strings.Split(strings.TrimPrefix(request.URL.Path, mediaRoutePrefix), "/")
		if !strings.HasPrefix(request.URL.Path, mediaRoutePrefix) || len(parts) != 3 ||
			!itemIDPattern.MatchString(parts[2]) {
			http.NotFound(writer, request)

			return
		}

		var item library.ContentItem

		storage.mediaMu.Lock()

		err := storage.db.View(func(transaction *bolt.Tx) error {
			card, err := storage.loadCard(transaction, parts[0], parts[1])
			if err != nil {
				return fmt.Errorf("media handler: %w", err)
			}

			for _, candidate := range card.Items {
				if candidate.ID == parts[2] && candidate.Kind != textKind {
					item = candidate

					return nil
				}
			}

			return errMediaNotFound
		})

		var file *os.File

		if err == nil {
			file, err = os.Open(storage.mediaPath(item.RelativePath))
		}

		storage.mediaMu.Unlock()

		if err != nil {
			http.NotFound(writer, request)

			return
		}

		defer file.Close()

		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(writer, request)

			return
		}

		writer.Header().Set("Content-Type", item.MIMEType)
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Cache-Control", "no-store")
		http.ServeContent(writer, request, item.OriginalName, info.ModTime(), file)
	})
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate Raven configuration directory: %w", err)
	}

	return filepath.Join(dir, "raven", "raven.db"), nil
}

func Open(path string, scheduler library.Scheduler) (*Store, error) {
	mkdirAllErr2 := os.MkdirAll(filepath.Dir(path), privateDirectoryMode)
	if mkdirAllErr2 != nil {
		return nil, fmt.Errorf("create database directory for %q: %w", path, mkdirAllErr2)
	}

	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}

	storage := &Store{
		db:        db,
		scheduler: scheduler,
		mediaDir:  filepath.Join(filepath.Dir(path), "media"),
	}

	initializeErr := storage.initialize(path)
	if initializeErr != nil {
		_ = db.Close()

		return nil, fmt.Errorf("initialize database %q: %w", path, initializeErr)
	}

	cleanupMediaErr := storage.cleanupMedia()
	if cleanupMediaErr != nil {
		_ = db.Close()

		return nil, fmt.Errorf("check media for %q: %w", path, cleanupMediaErr)
	}

	return storage, nil
}

func (storage *Store) Close() error {
	storage.mediaMu.Lock()
	defer storage.mediaMu.Unlock()

	forwardErr2 := storage.db.Close()
	if forwardErr2 != nil {
		return fmt.Errorf("close: %w", forwardErr2)
	}

	return nil
}

func (storage *Store) ListFolders() ([]library.Folder, error) {
	folders := make([]library.Folder, 0)

	err := storage.db.View(func(transaction *bolt.Tx) error {
		root := transaction.Bucket([]byte(foldersBucket))
		if root == nil {
			return errMissingFoldersBucket
		}

		return root.ForEach(func(key, value []byte) error {
			bucket := root.Bucket(key)
			if value != nil || bucket == nil || bucket.Bucket([]byte(cardsBucket)) == nil {
				return fmt.Errorf("invalid folder bucket %q", key)
			}

			var folder library.Folder

			err := decodeRecord(bucket.Get([]byte(metadataKey)), &folder)
			if err != nil {
				return fmt.Errorf("read folder %q: %w", key, err)
			}

			if folder.ID != string(key) || strings.TrimSpace(folder.Title) == "" {
				return fmt.Errorf("invalid metadata for folder %q", key)
			}

			if folder.Tags == nil {
				folder.Tags = []string{}
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

func (storage *Store) CreateFolder(input library.CreateFolderInput) (library.Folder, error) {
	folder, err := library.NormalizeFolder(input)
	if err != nil {
		return library.Folder{}, fmt.Errorf("create folder: %w", err)
	}

	err = storage.db.Update(func(transaction *bolt.Tx) error {
		root := transaction.Bucket([]byte(foldersBucket))
		if root == nil {
			return errMissingFoldersBucket
		}

		sequence, nextSequenceErr := root.NextSequence()
		if nextSequenceErr != nil {
			return fmt.Errorf("create folder: %w", nextSequenceErr)
		}

		if sequence == 0 {
			return errFolderIDSequenceExhausted
		}

		folder.ID = fmt.Sprintf(recordIDFormat, sequence)

		bucket, nextSequenceErr := root.CreateBucket([]byte(folder.ID))
		if nextSequenceErr != nil {
			return fmt.Errorf("create folder: %w", nextSequenceErr)
		}

		metadata, nextSequenceErr := json.Marshal(folder)
		if nextSequenceErr != nil {
			return fmt.Errorf("create folder: %w", nextSequenceErr)
		}

		putErr2 := bucket.Put([]byte(metadataKey), metadata)
		if putErr2 != nil {
			return fmt.Errorf("create folder: %w", putErr2)
		}

		_, nextSequenceErr = bucket.CreateBucket([]byte(cardsBucket))
		if nextSequenceErr != nil {
			return fmt.Errorf("create folder: %w", nextSequenceErr)
		}

		_, nextSequenceErr = bucket.CreateBucket([]byte(itemsBucket))
		if nextSequenceErr != nil {
			return fmt.Errorf("create folder: %w", nextSequenceErr)
		}

		return nil
	})
	if err != nil {
		return library.Folder{}, fmt.Errorf("create folder: %w", err)
	}

	return folder, nil
}

func (storage *Store) DeleteCard(folderID, cardID string) error {
	storage.mediaMu.Lock()
	defer storage.mediaMu.Unlock()

	err := storage.db.Update(func(transaction *bolt.Tx) error {
		_, loadCardErr4 := storage.loadCard(transaction, folderID, cardID)
		if loadCardErr4 != nil {
			return fmt.Errorf("delete card: %w", loadCardErr4)
		}

		folder := transaction.Bucket([]byte(foldersBucket)).Bucket([]byte(folderID))

		err := folder.Bucket([]byte(itemsBucket)).DeleteBucket([]byte(cardID))
		if err != nil {
			return fmt.Errorf("delete card: %w", err)
		}

		return folder.Bucket([]byte(cardsBucket)).Delete([]byte(cardID))
	})
	if err != nil {
		return fmt.Errorf("delete card: %w", err)
	}

	// Any failed file removal is retried by startup cleanup.
	_ = storage.cleanupMedia()

	return nil
}

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

	_ = storage.cleanupMedia()

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
		return library.Card{}, fmt.Errorf("load card: %w", err)
	}

	data := bucket.Get([]byte(cardID))
	if data == nil {
		return library.Card{}, errCardNotFound
	}

	var card library.Card

	decodeRecordErr3 := decodeRecord(data, &card)
	if decodeRecordErr3 != nil {
		return library.Card{}, fmt.Errorf("load card: %w", decodeRecordErr3)
	}

	if card.ID != cardID || card.FolderID != folderID || card.CreatedAt <= 0 {
		return library.Card{}, errInvalidCardMetadata
	}

	items := transaction.Bucket([]byte(foldersBucket)).
		Bucket([]byte(folderID)).
		Bucket([]byte(itemsBucket)).
		Bucket([]byte(cardID))
	if items == nil {
		return library.Card{}, errMissingCardContent
	}

	card.Items = make([]library.ContentItem, 0)

	err = items.ForEach(func(id, data []byte) error {
		var item library.ContentItem

		decodeRecordErr4 := decodeRecord(data, &item)
		if decodeRecordErr4 != nil {
			return fmt.Errorf("load card: %w", decodeRecordErr4)
		}

		if item.ID != string(id) || item.CardID != cardID || item.FolderID != folderID {
			return errInvalidContentOwnership
		}

		decodeRecordErr4 = library.ValidateItem(item)
		if decodeRecordErr4 != nil {
			return fmt.Errorf("load card: %w", decodeRecordErr4)
		}

		if meta := transaction.Bucket(
			[]byte(metaBucket),
		); meta != nil &&
			(string(meta.Get([]byte(schemaKey))) == schemaV4 || string(meta.Get([]byte(schemaKey))) == schemaV5) {
			if item.Schedule.Due.IsZero() || item.Schedule.LastReview.IsZero() ||
				len(item.Reviews) == 0 {
				return errMissingItemSchedule
			}

			validateErr := storage.scheduler.Validate(item.Schedule)
			if validateErr != nil {
				return fmt.Errorf("invalid item schedule: %w", validateErr)
			}
		}

		card.Items = append(card.Items, item)

		return nil
	})
	if err != nil {
		return library.Card{}, fmt.Errorf("load card: %w", err)
	}

	result2, forwardErr3 := library.Hydrate(card)
	if forwardErr3 != nil {
		return result2, fmt.Errorf("load card: %w", forwardErr3)
	}

	return result2, nil
}

func putCard(bucket *bolt.Bucket, card library.Card) error {
	card.Items = nil

	data, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("put card: %w", err)
	}

	forwardErr6 := bucket.Put([]byte(card.ID), data)
	if forwardErr6 != nil {
		return fmt.Errorf("put card: %w", forwardErr6)
	}

	return nil
}

func putItems(transaction *bolt.Tx, card library.Card) error {
	root := transaction.Bucket([]byte(foldersBucket)).
		Bucket([]byte(card.FolderID)).
		Bucket([]byte(itemsBucket))
	if root.Bucket([]byte(card.ID)) != nil {
		err := root.DeleteBucket([]byte(card.ID))
		if err != nil {
			return fmt.Errorf("put items: %w", err)
		}
	}

	bucket, err := root.CreateBucket([]byte(card.ID))
	if err != nil {
		return fmt.Errorf("put items: %w", err)
	}

	for _, item := range card.Items {
		data, marshalErr := json.Marshal(item)
		if marshalErr != nil {
			return fmt.Errorf("put items: %w", marshalErr)
		}

		putErr3 := bucket.Put([]byte(item.ID), data)
		if putErr3 != nil {
			return fmt.Errorf("put items: %w", putErr3)
		}
	}

	return nil
}

func (storage *Store) ListCards() ([]library.Card, error) {
	cards := make([]library.Card, 0)

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

func (storage *Store) CreateCard(input library.CreateCardInput) (library.Card, error) {
	result5, forwardErr7 := storage.saveCard(input.FolderID, "", input.Items)
	if forwardErr7 != nil {
		return result5, fmt.Errorf("create card: %w", forwardErr7)
	}

	return result5, nil
}

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

	var previous library.Card

	viewErr := storage.db.View(func(transaction *bolt.Tx) error {
		if cardID == "" {
			_, err := cardBucket(transaction, folderID)
			if err != nil {
				return fmt.Errorf("save card: %w", err)
			}

			return nil
		}

		var err error

		previous, err = storage.loadCard(transaction, folderID, cardID)
		if err != nil {
			return fmt.Errorf("save card: %w", err)
		}

		return nil
	})
	if viewErr != nil {
		return library.Card{}, fmt.Errorf("save card: %w", viewErr)
	}

	items, imported, err := library.PrepareItems(
		drafts,
		previous.Items,
		storage.importMedia,
		newItemID,
	)
	committed := false

	defer func() {
		if !committed {
			for _, path := range imported {
				_ = os.Remove(path)
			}
		}
	}()

	if err != nil {
		return library.Card{}, fmt.Errorf("save card: %w", err)
	}

	var card library.Card

	err = storage.db.Update(func(transaction *bolt.Tx) error {
		bucket, cardBucketErr := cardBucket(transaction, folderID)
		if cardBucketErr != nil {
			return fmt.Errorf("save card: %w", cardBucketErr)
		}

		if cardID == "" {
			sequence, nextSequenceErr2 := bucket.NextSequence()
			if nextSequenceErr2 != nil {
				return fmt.Errorf("save card: %w", nextSequenceErr2)
			}

			if sequence == 0 {
				return errCardIDSequenceExhausted
			}

			now := time.Now().UnixMilli()

			card = library.Card{
				ID:        fmt.Sprintf(recordIDFormat, sequence),
				FolderID:  folderID,
				CreatedAt: now,
				DueAt:     now,
			}

			if bucket.Get([]byte(card.ID)) != nil {
				return errCardIDAlreadyExists
			}
		} else {
			// Read metadata again so a review during file copying is preserved.
			card, cardBucketErr = storage.loadCard(transaction, folderID, cardID)
			if cardBucketErr != nil {
				return fmt.Errorf("save card: %w", cardBucketErr)
			}
		}

		// Merge fresh scheduling metadata inside the write transaction, preserving
		// reviews and notification acknowledgments made during media imports.
		fresh := make(map[string]library.ContentItem)

		for _, item := range card.Items {
			fresh[item.ID] = item
		}

		card.Items = append([]library.ContentItem(nil), items...)

		initializedAt := time.Now()

		for i := range card.Items {
			item := &card.Items[i]
			if old, ok := fresh[item.ID]; ok {
				item.Schedule = old.Schedule
				item.Reviews = old.Reviews
				item.LastNotifiedDue = old.LastNotifiedDue
			} else {
				initializeItemErr2 := library.InitializeItem(item, initializedAt, storage.scheduler)
				if initializeItemErr2 != nil {
					return fmt.Errorf("save card: %w", initializeItemErr2)
				}
			}
		}

		for i := range card.Items {
			card.Items[i].FolderID = folderID
			card.Items[i].CardID = card.ID
		}

		putItemsErr2 := putItems(transaction, card)
		if putItemsErr2 != nil {
			return fmt.Errorf("save card: %w", putItemsErr2)
		}

		putCardErr := putCard(bucket, card)
		if putCardErr != nil {
			return fmt.Errorf("save card: %w", putCardErr)
		}

		card, cardBucketErr = storage.loadCard(transaction, folderID, card.ID)
		if cardBucketErr != nil {
			return fmt.Errorf("save card: %w", cardBucketErr)
		}

		return nil
	})
	if err != nil {
		return library.Card{}, fmt.Errorf("save card: %w", err)
	}

	committed = true

	kept := make(map[string]bool)

	for _, item := range card.Items {
		kept[item.ID] = true
	}

	for _, item := range previous.Items {
		if item.Kind != textKind && !kept[item.ID] {
			_ = os.Remove(storage.mediaPath(item.RelativePath))
		}
	}

	return card, nil
}

func (storage *Store) ReviewItem(
	folderID, cardID, itemID string,
	rating int,
) (library.Card, error) {
	result7, forwardErr9 := storage.reviewItemAt(folderID, cardID, itemID, rating, time.Time{})
	if forwardErr9 != nil {
		return result7, fmt.Errorf("review item: %w", forwardErr9)
	}

	return result7, nil
}

func (storage *Store) reviewItemAt(
	folderID, cardID, itemID string,
	rating int,
	now time.Time,
) (library.Card, error) {
	if rating < 1 || rating > 4 {
		return library.Card{}, errInvalidReviewRating
	}

	var card library.Card

	err := storage.db.Update(func(transaction *bolt.Tx) error {
		var err error

		card, err = storage.loadCard(transaction, folderID, cardID)
		if err != nil {
			return fmt.Errorf("review item at: %w", err)
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
				return fmt.Errorf("review item at: %w", applyReviewErr)
			}

			found = true
		}

		if !found {
			return errContentItemNotFound
		}

		putItemsErr3 := putItems(transaction, card)
		if putItemsErr3 != nil {
			return fmt.Errorf("review item at: %w", putItemsErr3)
		}

		card, err = storage.loadCard(transaction, folderID, cardID)
		if err != nil {
			return fmt.Errorf("review item at: %w", err)
		}

		return nil
	})
	if err != nil {
		return library.Card{}, fmt.Errorf("review item: %w", err)
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
	if len(trimmed) == 0 {
		return nil, errEmptyStoredRecord
	}

	switch trimmed[0] {
	case '{':
		result4, forwardErr5 := normalizeObject(trimmed)
		if forwardErr5 != nil {
			return result4, fmt.Errorf("normalize record: %w", forwardErr5)
		}

		return result4, nil
	case '[':
		result3, forwardErr4 := normalizeArray(trimmed)
		if forwardErr4 != nil {
			return result3, fmt.Errorf("normalize record: %w", forwardErr4)
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
	case "sourcePath":
		return "source_path"
	case "originalName":
		return "original_name"
	case "previewURL":
		return "preview_url"
	case "byteSize":
		return "byte_size"
	case "folderId":
		return "folder_id"
	case "cardId":
		return "card_id"
	case "createdAt":
		return "created_at"
	case "dueAt":
		return "due_at"
	case "lastReviewed":
		return "last_reviewed"
	case "mimeType":
		return "mime_type"
	case "relativePath":
		return "relative_path"
	case "lastNotifiedDue":
		return "last_notified_due"
	case "Due":
		return "due"
	case "LastReview":
		return "last_review"
	case "Stability":
		return "stability"
	case "Difficulty":
		return "difficulty"
	case "ScheduledDays":
		return "scheduled_days"
	case "Reps":
		return "reps"
	case "Lapses":
		return "lapses"
	case "RemainingSteps":
		return "remaining_steps"
	case "State":
		return "state"
	case "Review":
		return "review"
	case "Rating":
		return "rating"
	default:
		return key
	}
}
