// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package composition

import (
	"go.uber.org/fx"

	"raven/internal/features/folderarchive/adapters/zipfile"
	archiveapplication "raven/internal/features/folderarchive/application"
	"raven/internal/features/folderarchive/ports"
)

// Module wires folder archive workflows and their adapters.
//
//nolint:gochecknoglobals // Fx modules are immutable dependency registration declarations.
var Module = fx.Module("folderarchive", fx.Provide(
	fx.Annotate(archiveLibrary, fx.As(new(ports.Library))),
	fx.Annotate(zipfile.New, fx.As(new(ports.Archives))),
	fx.Annotate(archiveapplication.New, fx.As(new(ports.Transfer))),
))
