//go:build !windows

// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package retry

func platformTransient(_ error) bool { return false }
