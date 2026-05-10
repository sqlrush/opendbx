// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Profile checkpoint relay (spec-0.3 D-9 R2 fixup).
//
// `cmd/opendbx/main.go` records its first checkpoint via this relay so the
// only cmd → platform exception remains `internal/platform/version` (spec-0.2
// § 2.2). The relay forwards to internal/platform/profileutil; entrypoints
// is allowed to import platform/* under the existing layer matrix.

package entrypoints

import (
	"io"

	"github.com/sqlrush/opendbx/internal/platform/profileutil"
)

// Checkpoint records a startup checkpoint. Equivalent to
// profileutil.Checkpoint, but reachable from cmd/opendbx without violating
// the cmd → platform/version unique-exception rule (CC main.tsx L1
// `profileCheckpoint('main_tsx_entry')` parity).
func Checkpoint(name string) {
	profileutil.Checkpoint(name)
}

// ReportProfile writes the profile report when `--debug=profile` is set
// (spec § 7 DoD D-9). No-op when the report contains zero entries.
func ReportProfile(w io.Writer) {
	profileutil.Report(w)
}
