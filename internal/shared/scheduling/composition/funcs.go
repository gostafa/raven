// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	configuration "raven/internal/features/configuration/domain"
	scheduling "raven/internal/shared/scheduling/adapters/fsrs"
)

func newScheduler(settings *configuration.Settings) scheduling.Scheduler {
	return scheduling.New(&settings.Library)
}
