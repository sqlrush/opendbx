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

	"github.com/sqlrush/opendbx/internal/bootstrap"
)

// ErrInteractiveHelperNotImplemented signals an interactive helper has not
// yet been wired to a real React/Ink-equivalent UI. Replaced in
// spec-1.15-tui. Moved to errors.go in spec-0.6 D-4 (now carries
// Code/Message/Hint via errcode registry).

// RenderAndRun parallels CC interactiveHelpers.ts::renderAndRun. spec-0.12
// D-4 wires it to the empty tcell main loop via bootstrap.LaunchInteractiveTUI
// (layer chain entrypoints → bootstrap → app/cli/tui; cmd cannot reach app
// directly). spec-1.15-tui replaces the empty loop body with the real
// render engine + dispatcher.
func RenderAndRun(ctx context.Context) error {
	return bootstrap.LaunchInteractiveTUI(ctx)
}

// ShowSetupDialog parallels CC interactiveHelpers.ts::showSetupDialog.
// Returns the dialog result (typed as `any` until spec-1.15 binds it to a
// concrete dialog-result type).
//
// Stage-0: returns ErrInteractiveHelperNotImplemented.
func ShowSetupDialog(_ context.Context, _ any) (any, error) {
	return nil, ErrInteractiveHelperNotImplemented
}
