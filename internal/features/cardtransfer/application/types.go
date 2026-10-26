// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package application coordinates card transfers.
package application

// Service validates transfer requests before calling persistence.
type (
	Service struct{ execute func(string) (string, error) }
)
