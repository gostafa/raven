// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

// Package native adapts native catalogs to consumer-owned message ports.
package native

type (
	renderRequest = struct {
		locale string
		key    string
		count  int
	}
	// Messages resolves native strings and language lifecycle operations.
	Messages struct {
		operations map[string]func(string, int, func()) nativeResult
	}
	nativeResult = struct {
		cancel func()
		text   string
	}
)
