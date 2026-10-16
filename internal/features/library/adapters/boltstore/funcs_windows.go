//go:build windows

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package boltstore

func syncDirectory(_ string) error { return nil }
