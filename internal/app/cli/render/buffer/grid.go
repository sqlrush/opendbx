// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package buffer

import (
	"math"

	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

// Grid is the production concrete Buffer (spec-1.2 D-1).
//
// Storage: row-major flat backing array `cells[y*cols + x]` for cache
// locality (a `[][]Cell` 2D slice fragments the heap and pessimises
// the per-frame scan loop in spec-1.3 optimizer). A parallel
// `cellGen []uint64` carries per-cell generation stamps so Reset can
// be O(1) (see reset.go) without touching the Cell struct sizeof.
//
// Concurrency: NOT safe for concurrent use. spec-0.13 D-3 single-owner
// contract; the render goroutine (spec-1.4 scheduler frame loop) owns
// a Grid for its lifetime. Cross-goroutine access requires caller-side
// synchronization.
//
// Ownership lifetime (spec-1.2 R2 D3): when obtained via
// BufferPool.Acquire, the caller owns the returned *Grid until
// Release. See BufferPool godoc for the use-after-release anti-pattern.
type Grid struct {
	cols, rows int
	cells      []Cell
	cellGen    []uint64
	generation uint64
}

// Compile-time assertion that *Grid satisfies Buffer.
var _ Buffer = (*Grid)(nil)

// NewGrid constructs a fresh Grid with the given (cols, rows). Both
// must be > 0 and their product must fit in int32 (≈ 2.1×10^9 cells,
// far above any practical viewport). Returns nil and ErrInvalidDimension
// for invalid input.
//
// The generation counter starts at 1 (not 0) so a freshly allocated
// cellGen slice (zero-valued) never spuriously matches a "live" cell —
// see reset.go and § 13 附录 B of spec-1.2.
func NewGrid(cols, rows int) (*Grid, error) {
	if cols <= 0 || rows <= 0 {
		return nil, ErrInvalidDimension
	}
	if int64(cols)*int64(rows) > int64(math.MaxInt32) {
		return nil, ErrInvalidDimension
	}
	n := cols * rows
	return &Grid{
		cols:       cols,
		rows:       rows,
		cells:      make([]Cell, n),
		cellGen:    make([]uint64, n),
		generation: 1,
	}, nil
}

// Cell returns the cell at (x, y). Out-of-bounds coordinates and stale
// cells (whose generation does not match the current grid generation)
// return the zero Cell{}; this is the post-Reset "cleared" state. The
// spec-0.13 D-3 Buffer contract forbids panicking the render goroutine
// on out-of-bounds reads.
func (g *Grid) Cell(x, y int) Cell {
	if x < 0 || x >= g.cols || y < 0 || y >= g.rows {
		return Cell{}
	}
	idx := y*g.cols + x
	if g.cellGen[idx] != g.generation {
		return Cell{}
	}
	return g.cells[idx]
}

// SetCell writes c at (x, y) and, if c.Ch is an East Asian wide rune
// (RuneWidth == 2), writes a continuation cell at (x+1, y) carrying
// Cell{Ch: WideContinuation, St: c.St}. The continuation write goes
// through the private setCellRaw helper to avoid re-running width
// detection (and double-writing further continuations).
//
// Edge cases:
//   - out-of-bounds (x, y) is a silent no-op (no panic per spec-0.13
//     D-3 contract; callers should Size()-clamp before writing);
//   - wide rune at the last column (x == cols-1) writes only the main
//     cell — the continuation is dropped silently. Higher-level
//     measure logic (spec-1.7 block) should clamp via width.Width
//     before reaching this code path.
func (g *Grid) SetCell(x, y int, c Cell) {
	if x < 0 || x >= g.cols || y < 0 || y >= g.rows {
		return
	}
	idx := y*g.cols + x
	g.cells[idx] = c
	g.cellGen[idx] = g.generation
	if width.RuneWidth(c.Ch) == 2 && x+1 < g.cols {
		g.setCellRaw(x+1, y, Cell{Ch: WideContinuation, St: c.St})
	}
}

// setCellRaw writes c at (x, y) without running width detection.
// Private helper used by SetCell to place continuation cells; the
// caller has already validated (x, y) is in bounds.
func (g *Grid) setCellRaw(x, y int, c Cell) {
	idx := y*g.cols + x
	g.cells[idx] = c
	g.cellGen[idx] = g.generation
}

// Size returns the current dimensions.
func (g *Grid) Size() (cols, rows int) {
	return g.cols, g.rows
}

// Resize changes the dimensions of the Grid. Per the spec-0.13 D-3
// destructive contract:
//
//   - cells inside min(oldCols,newCols) × min(oldRows,newRows) are
//     preserved at their (x, y) coordinates;
//   - cells outside the new bounds are discarded;
//   - cellGen[i] for surviving cells is preserved verbatim (their
//     generation stamps remain valid relative to g.generation; spec-1.2
//     R2 claude MED-3).
//
// When cols changes, we MUST per-row copy (a flat reslice would
// shift (x, y) coordinates). Only when cols is unchanged and rows
// shrinks may we reslice in place.
//
// Wide-rune shrink-clip (spec-1.2 R2 MED-3): if a wide rune's main
// cell survives at (newCols-1, y) but its continuation at newCols
// would fall outside the new grid, the main cell is preserved and
// the continuation is dropped. Callers must re-measure layout after
// Resize to avoid visual half-glyphs in narrowed viewports.
//
// Invalid (cols, rows) (≤ 0 or overflowing int32) are silently
// rejected — the spec-0.13 D-3 Buffer interface returns void from
// Resize, so we cannot surface ErrInvalidDimension here. Callers
// must validate dimensions before calling Resize.
func (g *Grid) Resize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	if int64(cols)*int64(rows) > int64(math.MaxInt32) {
		return
	}
	if cols == g.cols && rows == g.rows {
		return
	}
	// Fast path: cols unchanged AND rows shrinking, AND we have spare
	// capacity. Reslice in place; the surviving cells keep (x, y).
	if cols == g.cols && rows < g.rows {
		n := cols * rows
		g.cells = g.cells[:n]
		g.cellGen = g.cellGen[:n]
		g.rows = rows
		return
	}
	// General path: allocate new backing arrays and per-row copy the
	// preserved region. cellGen carries verbatim so existing-generation
	// reads stay valid.
	n := cols * rows
	newCells := make([]Cell, n)
	newGen := make([]uint64, n)
	copyCols := cols
	if g.cols < copyCols {
		copyCols = g.cols
	}
	copyRows := rows
	if g.rows < copyRows {
		copyRows = g.rows
	}
	for y := 0; y < copyRows; y++ {
		srcStart := y * g.cols
		dstStart := y * cols
		copy(newCells[dstStart:dstStart+copyCols], g.cells[srcStart:srcStart+copyCols])
		copy(newGen[dstStart:dstStart+copyCols], g.cellGen[srcStart:srcStart+copyCols])
	}
	g.cells = newCells
	g.cellGen = newGen
	g.cols = cols
	g.rows = rows
}
