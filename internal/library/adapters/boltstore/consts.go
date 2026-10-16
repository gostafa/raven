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
)
