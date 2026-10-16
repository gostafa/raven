// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import "go.uber.org/fx"

var Module = fx.Module("companion", fx.Provide(newConfiguration, newWindows))
