// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package domain

import (
	transferapi "raven/api/cardtransfer"
)

// Normalize validates the operation and removes repeated source references.
func Normalize(input *transferapi.TransferInput) error {
	if !validRequest(input) {
		return errInvalidTransfer
	}

	sources, err := uniqueSources(input.Sources)
	if err != nil {
		return errInvalidTransfer
	}

	input.Sources = sources

	return nil
}

func validRequest(input *transferapi.TransferInput) bool {
	return validOperation(input.Operation) && input.DestinationFolderID != "" &&
		len(input.Sources) > zero
}

func validOperation(operation string) bool { return operation == Copy || operation == Move }

func uniqueSources(input []transferapi.Reference) ([]transferapi.Reference, error) {
	seen := make(map[transferapi.Reference]bool, len(input))
	sources := make([]transferapi.Reference, zero, len(input))

	for index := range input {
		err := appendSource(&sources, seen, &input[index])
		if err != nil {
			return nil, errInvalidTransfer
		}
	}

	return sources, nil
}

func appendSource(
	sources *[]transferapi.Reference,
	seen map[transferapi.Reference]bool,
	source *transferapi.Reference,
) error {
	if source.FolderID == "" || source.CardID == "" {
		return errInvalidTransfer
	}

	if !seen[*source] {
		*sources = append(*sources, *source)
		seen[*source] = true
	}

	return nil
}
