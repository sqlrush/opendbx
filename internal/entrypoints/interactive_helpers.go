// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Stage-0 interactive helper stubs (spec-0.3 D-5 + spec § 2.2 file-layout
// contract).
//
// Mirrors CC src/interactiveHelpers.ts (renderAndRun, showSetupDialog) —
// the function signatures parallel CC's React/Ink contract so that
// spec-1.15-tui can swap in the real implementations without ripple changes
// to call sites.

package entrypoints

import (
	"context"
	"errors"
)

// ErrInteractiveHelperNotImplemented signals an interactive helper has not
// yet been wired to a real React/Ink-equivalent UI. Replaced in
// spec-1.15-tui.
var ErrInteractiveHelperNotImplemented = errors.New("interactive helper not implemented in stage 0 (lands in spec-1.15-tui)")

// RenderAndRun parallels CC interactiveHelpers.ts::renderAndRun. Real
// implementation drives the tcell event loop + render engine to produce a
// React-equivalent re-render cycle.
//
// Stage-0: returns ErrInteractiveHelperNotImplemented.
func RenderAndRun(_ context.Context) error {
	return ErrInteractiveHelperNotImplemented
}

// ShowSetupDialog parallels CC interactiveHelpers.ts::showSetupDialog.
// Returns the dialog result (typed as `any` until spec-1.15 binds it to a
// concrete dialog-result type).
//
// Stage-0: returns ErrInteractiveHelperNotImplemented.
func ShowSetupDialog(_ context.Context, _ any) (any, error) {
	return nil, ErrInteractiveHelperNotImplemented
}
