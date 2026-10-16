// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package library

import (
	"errors"
	"regexp"
)

var (
	itemIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

	errInvalidContentItem             = errors.New("invalid content item")
	errInvalidTextItem                = errors.New("invalid text item")
	errInvalidContentKind             = errors.New("invalid content kind")
	errInvalidMediaItem               = errors.New("invalid media item")
	errEnterAFolderTitle              = errors.New("enter a folder title")
	errInvalidContentOrder            = errors.New("invalid content order")
	errAddSomethingYouWantToRemember  = errors.New("add something you want to remember")
	errInvalidReviewRating            = errors.New("invalid review rating")
	errInvalidExistingItemOrOwnership = errors.New("invalid existing item or ownership")
	errTextCannotReferenceAFile       = errors.New("text cannot reference a file")
	errMediaCannotContainText         = errors.New("media cannot contain text")
)
