// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package buffer is the cell-grid data structure for the render engine.
// A Buffer holds a 2D Grid of Cells (rune + style). spec-1.3-cell-grid-buffer
// adds sync.Pool + generational reset; spec-0.13 D-1 provides only the
// interface skeleton.
//
// DAG position: render/buffer is index 3 (depends on render/style + render/width).
//
// Design: spec-0.13-render-engine-skeleton § 2.1 (D-1)

package buffer

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
)

// Cell is a single grid cell: a rune + its style.
type Cell struct {
	Ch rune
	St style.Style
}

// Buffer is the abstract cell grid. Concrete implementations (allocating
// vs pooled) come in spec-1.3.
type Buffer interface {
	Cell(x, y int) Cell
	SetCell(x, y int, c Cell)
	Size() (cols, rows int)
	Resize(cols, rows int)
}
