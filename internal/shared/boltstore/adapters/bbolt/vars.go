// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package bbolt

import (
	"errors"
	"os"
	"regexp"

	bolt "go.etcd.io/bbolt"

	autostartports "raven/internal/features/autostart/ports"
	reminders "raven/internal/features/reminders/application"
	libraryports "raven/internal/shared/boltstore/ports"
)

var (
	_                      database    = (*bolt.DB)(nil)
	_                      snapshot    = (*bolt.Tx)(nil)
	_                      durableFile = (*os.File)(nil)
	errStoreAlreadyStarted             = errors.New(
		"library persistence already started",
	)
	_ libraryports.Repository = (*Store)(nil)
	_ autostartports.Settings = (*Store)(nil)
	_ reminders.Source        = (*Store)(nil)

	// Deliberately exclude active formats such as SVG and HTML.

	itemIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

	errMissingMetadata                     = errors.New("missing metadata")
	errMissingFoldersBucket                = errors.New("missing folders bucket")
	errInvalidLegacyFolder                 = errors.New("invalid legacy folder")
	errMissingLegacyContentBucket          = errors.New("missing legacy content bucket")
	errMissingLegacyCardContent            = errors.New("missing legacy card content")
	errInvalidLegacyContentOwnershipOrSide = errors.New(
		"invalid legacy content ownership or side",
	)
	errInvalidLegacyContentOrder        = errors.New("invalid legacy content order")
	errChooseASupportedFileForThisMedia = errors.New(
		"choose a supported file for this media type",
	)
	errEmptyMediaFile                          = errors.New("empty media file")
	errSelectARegularMediaFile                 = errors.New("select a regular media file")
	errSelectedFileChangedDuringImportTryAgain = errors.New(
		"selected file changed during import; try again",
	)
	errMediaNotFound             = errors.New("media not found")
	errFolderIDSequenceExhausted = errors.New("folder ID sequence exhausted")
	errFolderNotFound            = errors.New("folder not found")
	errFolderNotFoundOrInvalid   = errors.New("folder not found or invalid")
	errCardNotFound              = errors.New("card not found")
	errInvalidCardMetadata       = errors.New("invalid card metadata")
	errMissingCardContent        = errors.New("missing card content")
	errInvalidContentOwnership   = errors.New("invalid content ownership")
	errMissingItemSchedule       = errors.New("missing item schedule")
	errInvalidFolderBucket       = errors.New("invalid folder bucket")
	errCardIDIsRequired          = errors.New("card ID is required")
	errCardIDSequenceExhausted   = errors.New("card ID sequence exhausted")
	errCardIDAlreadyExists       = errors.New("card ID already exists")
	errInvalidReviewRating       = errors.New("invalid review rating")
	errContentItemNotFound       = errors.New("content item not found")

	errEmptyStoredRecord    = errors.New("empty stored record")
	errAmbiguousStoredField = errors.New("ambiguous stored field names")

	errUnsupportedDatabaseSchema  = errors.New("unsupported database schema")
	errIncompleteMigrationBackup  = errors.New("incomplete migration backup")
	errInvalidLegacyCardOwnership = errors.New("invalid legacy card ownership")
	errInvalidLegacyCard          = errors.New("invalid legacy card")

	errMediaContentTypeMismatch = errors.New("file contents do not match")

	errInvalidFolderMetadata = errors.New("invalid metadata for folder")
)
