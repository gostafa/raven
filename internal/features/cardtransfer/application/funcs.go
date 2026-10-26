// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package application

import (
	"encoding/json"
	"errors"
	"fmt"

	transferapi "raven/api/cardtransfer"
	"raven/internal/features/cardtransfer/domain"
	"raven/internal/features/cardtransfer/ports"
)

// New constructs the transfer workflow without acquiring resources.
func New(library ports.Library) *Service {
	return &Service{
		execute: func(input string) (string, error) { return transferCards(library, input) },
	}
}

// TransferCards returns source-to-destination mappings after an atomic transfer.
func (service *Service) TransferCards(input string) (string, error) {
	result, err := service.execute(input)

	return result, errors.Join(err)
}

func transferCards(library ports.Library, input string) (string, error) {
	err := validateRequest(input)
	if err != nil {
		return "", fmt.Errorf("validate transfer: %w", err)
	}

	result, err := library.ApplyCardTransfer(input)

	return result, errors.Join(err)
}

func validateRequest(input string) error {
	var request transferapi.TransferInput

	err := json.Unmarshal([]byte(input), &request)
	if err != nil {
		return fmt.Errorf("decode transfer: %w", err)
	}

	return errors.Join(domain.Normalize(&request))
}
