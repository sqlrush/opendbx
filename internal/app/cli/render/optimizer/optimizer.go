// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package optimizer diffs prev/next buffer.Buffer snapshots and emits
// minimal Patches for the terminal driver. spec-0.13 D-1 ships interface
// only; spec-1.3 implements the production DiffEngine (naive
// cell-by-cell row-major scan + IsContinuation skip + PatchResize on
// size mismatch + nil-prev fullRedraw fast path).
//
// DAG position: render/optimizer is index 5; in practice this package
// only imports render/buffer (3). Patch is data — ANSI escape generation
// is the terminal.Driver (tcell-backed, spec-1.4) responsibility.
//
// Design: spec-0.13-render-engine-skeleton § 2.1 (D-1) +
// spec-1.3-optimizer-ansi-patch (DiffEngine impl)
package optimizer

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// PatchKind tags the patch operation.
type PatchKind uint8

// Patch kinds enumerate the optimizer's minimal-change vocabulary.
//
// spec-1.3 R2 G2 hard guard: numeric values are part of the data
// contract and locked by TestPatchKinds_Values — do not rely on
// comments only. PatchMoveCursor / PatchStyleChange are reserved for
// spec-1.3a coalescing optimisation (cursor-move skips + SGR delta
// emission); the spec-1.3 DiffEngine never emits these two kinds.
const (
	PatchSetCell     PatchKind = iota // 0: overwrite a single cell at (X,Y)
	PatchMoveCursor                   // 1: reposition cursor only — reserved spec-1.3a
	PatchStyleChange                  // 2: change SGR without moving / writing — reserved spec-1.3a
	PatchResize                       // 3: terminal resize; consumes NewCols/NewRows (spec-1.3 D-3)
)

// Patch is a single minimal change to apply to the terminal driver.
type Patch struct {
	Kind    PatchKind
	X, Y    int
	Cell    buffer.Cell
	NewCols int // for terminal resize patches
	NewRows int
}

// Optimizer computes patches between two buffer snapshots.
type Optimizer interface {
	Diff(prev, next buffer.Buffer) []Patch
}
