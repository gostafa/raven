// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	contentapi "raven/api/content"
)

// Encode serializes a manifest without local filesystem capabilities.
func Encode(document *Document) (string, error) {
	data, err := json.Marshal(document)

	return string(data), errors.Join(err)
}

// Decode validates a portable manifest and preserves integer-valued scheduling state.
func Decode(manifest string) (*Document, error) {
	if !json.Valid([]byte(manifest)) || len(manifest) > MaximumManifestSize {
		return nil, ErrInvalidArchive
	}

	document, err := decodeDocument(manifest)
	if err != nil {
		return nil, errors.Join(err)
	}

	return document, nil
}

func decodeDocument(manifest string) (*Document, error) {
	var document Document

	err := manifestDecoder(manifest).Decode(&document)
	if err != nil {
		return nil, fmt.Errorf("decode folder manifest: %w", err)
	}

	validated, err := validatedDocument(&document)
	if err != nil {
		return nil, errors.Join(err)
	}

	return validated, nil
}

func manifestDecoder(manifest string) *json.Decoder {
	decoder := json.NewDecoder(bytes.NewBufferString(manifest))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()

	return decoder
}

func validatedDocument(document *Document) (*Document, error) {
	err := ValidateEnvelope(document)
	if err != nil {
		return nil, errors.Join(err)
	}

	return document, nil
}

// ValidateMode requires an explicit fresh or resume choice.
func ValidateMode(mode string) error {
	if mode != Fresh && mode != Resume {
		return ErrInvalidMode
	}

	return nil
}

// ValidateEnvelope checks archive identity before persistence validates its records.
func ValidateEnvelope(document *Document) error {
	if !validEnvelope(document) {
		return ErrInvalidArchive
	}

	return nil
}

func validEnvelope(document *Document) bool {
	return document.Version == Version && !document.ExportedAt.IsZero() &&
		len(document.Folders) != zero
}

// MediaName permits only flat, portable filenames in the media directory.
func MediaName(name string) bool {
	return strings.HasPrefix(name, "media/") && path.Dir(name) == "media" &&
		path.Base(name) != "." && path.Clean(name) == name &&
		!strings.ContainsAny(name, "\\:\x00")
}

// Summary counts the exported records without exposing local storage paths.
func Summary(document *Document) (Result, error) {
	result := Result{
		FolderIDs:   []string{},
		FolderCount: len(document.Folders),
		Filename:    "",
		CardCount:   zero,
		MediaCount:  zero,
		Canceled:    false,
	}
	for index := range document.Folders {
		err := countFolder(&result, &document.Folders[index])
		if err != nil {
			return Result{}, errors.Join(err)
		}
	}

	return result, nil
}

func countFolder(result *Result, folder *Folder) error {
	result.CardCount += len(folder.Cards)
	for index := range folder.Cards {
		items, err := contentapi.Decode[[]contentapi.Item](folder.Cards[index].Items)
		if err != nil {
			return errors.Join(ErrInvalidArchive, err)
		}

		countMedia(result, items)
	}

	return nil
}

func countMedia(result *Result, items []contentapi.Item) {
	for index := range items {
		if items[index].RelativePath != "" {
			result.MediaCount++
		}
	}
}
