// Package buffer is the fake fixture stand-in for the production
// internal/app/cli/render/buffer package — provides only the API
// surface paint-pattern-lint inspects (Cell type, Grid receiver,
// Buffer.Cell method, IsContinuation guard) so fixture .go files can
// compile + load with proper type info under packages.Load.
package buffer

// Cell mirrors the production buffer.Cell shape (lint only cares about
// the type identity, not the fields).
type Cell struct {
	Ch rune
}

// Buffer mirrors the production read-only interface; Cell() returns
// buffer.Cell so taint detection has type info to anchor on.
type Buffer interface {
	Size() (int, int)
	Cell(x, y int) Cell
}

// Grid mirrors the production *Grid receiver; SetCell takes a Cell.
type Grid struct {
	cols, rows int
	cells      []Cell
}

// NewGrid allocates a fresh grid (fixtures use it inline).
func NewGrid(cols, rows int) (*Grid, error) {
	return &Grid{cols: cols, rows: rows, cells: make([]Cell, cols*rows)}, nil
}

// Size reports dimensions.
func (g *Grid) Size() (int, int) { return g.cols, g.rows }

// Cell reads.
func (g *Grid) Cell(x, y int) Cell {
	if x < 0 || x >= g.cols || y < 0 || y >= g.rows {
		return Cell{}
	}
	return g.cells[y*g.cols+x]
}

// SetCell writes.
func (g *Grid) SetCell(x, y int, c Cell) {
	if x < 0 || x >= g.cols || y < 0 || y >= g.rows {
		return
	}
	g.cells[y*g.cols+x] = c
}

// IsContinuation mirrors the real guard predicate.
func IsContinuation(c Cell) bool { return c.Ch == -1 }
