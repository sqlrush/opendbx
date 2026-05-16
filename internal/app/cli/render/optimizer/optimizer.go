// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package optimizer diffs prev/next buffer.Buffer snapshots and emits
// minimal Patches (cursor moves + style changes + character writes) for
// the terminal driver. spec-0.13 D-1 ships interface only; spec-1.3+
// fill the algorithm.
//
// DAG position: render/optimizer is index 5 (depends on render/buffer +
// render/terminal).
//
// Design: spec-0.13-render-engine-skeleton § 2.1 (D-1)

package optimizer

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// PatchKind tags the patch operation.
type PatchKind uint8

const (
	PatchSetCell PatchKind = iota
	PatchMoveCursor
	PatchStyleChange
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
