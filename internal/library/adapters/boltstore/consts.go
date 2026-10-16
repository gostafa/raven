// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package boltstore

const (
	textKind                = "text"
	legacyPromptSide        = "prompt"
	legacyAnswerSide        = "answer"
	isoMediaSignature       = "ftyp"
	privateDirectoryMode    = 0o700
	mediaRoutePrefix        = "/media/"
	recordIDFormat          = "%020d"
	schemaV2                = "2"
	schemaV3                = "3"
	schemaV4                = "4"
	schemaV5                = "5"
	metaBucket              = "meta"
	schemaKey               = "schemaVersion"
	foldersBucket           = "folders"
	metadataKey             = "metadata"
	cardsBucket             = "cards"
	itemsBucket             = "card_items"
	settingsBucket          = "settings"
	autostartInitializedKey = "autostartInitialized"

	schemaV1 = "1"

	privateFileMode   = 0o600
	logErrorKey       = "error"
	mediaHeaderSize   = 512
	mimePartLimit     = 2
	isoHeaderSize     = 12
	isoSignatureStart = 4
	isoSignatureEnd   = 8

	claimAutostartInitializationErrorFormat = "claim autostart initialization: %w"
	cleanupMediaErrorFormat                 = "cleanup media: %w"
	createFolderErrorFormat                 = "create folder: %w"
	deleteCardErrorFormat                   = "delete card: %w"
	errorFormat                             = "%w %q"
	four                                    = 4
	importMediaErrorFormat                  = "import media: %w"
	loadCardErrorFormat                     = "load card: %w"
	markNotifiedErrorFormat                 = "mark notified: %w"
	mergeLegacyItemsErrorFormat             = "merge legacy items: %w"
	normalizeRecordErrorFormat              = "normalize record: %w"
	one                                     = 1
	putCardErrorFormat                      = "put card: %w"
	putItemsErrorFormat                     = "put items: %w"
	reviewItemAtErrorFormat                 = "review item at: %w"
	reviewItemErrorFormat                   = "review item: %w"
	saveCardErrorFormat                     = "save card: %w"
	zero                                    = 0

	legacySourcePath      = "sourcePath"
	legacyOriginalName    = "originalName"
	legacyPreviewURL      = "previewURL"
	legacyByteSize        = "byteSize"
	legacyMimeType        = "mimeType"
	legacyRelativePath    = "relativePath"
	legacyFolderID        = "folderId"
	legacyCardID          = "cardId"
	legacyCreatedAt       = "createdAt"
	legacyDueAt           = "dueAt"
	legacyLastReviewed    = "lastReviewed"
	legacyLastNotifiedDue = "lastNotifiedDue"
	legacyLastReview      = "LastReview"
	legacyScheduledDays   = "ScheduledDays"
	legacyRemainingSteps  = "RemainingSteps"
	legacyDue             = "Due"
	legacyStability       = "Stability"
	legacyDifficulty      = "Difficulty"
	legacyReps            = "Reps"
	legacyLapses          = "Lapses"
	legacyState           = "State"
	legacyReview          = "Review"
	legacyRating          = "Rating"
)
