// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// errcode-backed sentinels for the entrypoints package (spec-0.6 D-3+D-4).
// ErrLauncherNotImplemented and ErrInteractiveHelperNotImplemented were
// previously plain errors.New() sentinels in dialog_launchers.go and
// interactive_helpers.go; they now carry Code/Message/Hint via errcode
// registry while preserving sentinel name + errors.Is backward-compat.

package entrypoints

import "github.com/sqlrush/opendbx/internal/platform/errcode"

//nolint:gochecknoglobals // spec-0.6 § 2.2.1 canonical Register pattern
var (
	// ErrLauncherNotImplemented — dialog launchers (spec-1.15-tui target).
	ErrLauncherNotImplemented errcode.Sentinel = errcode.Register(
		"ENTRYPOINTS.LAUNCHER_NOT_IMPLEMENTED",
		"dialog launcher not implemented in stage 0",
		"this surface lands in spec-1.15-tui; check the launcher type and target spec",
	)

	// ErrInteractiveHelperNotImplemented — interactive helpers (spec-1.15-tui).
	ErrInteractiveHelperNotImplemented errcode.Sentinel = errcode.Register(
		"ENTRYPOINTS.INTERACTIVE_HELPER_NOT_IMPLEMENTED",
		"interactive helper not implemented in stage 0",
		"this surface lands in spec-1.15-tui; check the helper type and target spec",
	)
)
