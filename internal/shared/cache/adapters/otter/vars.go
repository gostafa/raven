// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package otter

import (
	"errors"

	"raven/internal/shared/cache/ports"
)

var (
	errOversized                               = errors.New("cache value exceeds capacity")
	_            ports.Chunks                  = (*Cache[string, []byte])(nil)
	_            ports.Loading[string, string] = (*Cache[string, string])(nil)
)
