// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package buffer

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
)

func TestCell_ZeroValue(t *testing.T) {
	t.Parallel()
	c := Cell{}
	if c.Ch != 0 {
		t.Errorf("zero Cell.Ch = %v, want 0", c.Ch)
	}
	if (c.St != style.Style{}) {
		t.Errorf("zero Cell.St = %+v, want zero", c.St)
	}
}

// fakeBuffer implements Buffer to verify the interface surface compiles
// and contract semantics (including Resize from R2 claude MED-1).
type fakeBuffer struct {
	cols, rows int
	cells      map[[2]int]Cell
}

func newFake(cols, rows int) *fakeBuffer {
	return &fakeBuffer{cols: cols, rows: rows, cells: map[[2]int]Cell{}}
}

func (b *fakeBuffer) Cell(x, y int) Cell       { return b.cells[[2]int{x, y}] }
func (b *fakeBuffer) SetCell(x, y int, c Cell) { b.cells[[2]int{x, y}] = c }
func (b *fakeBuffer) Size() (int, int)         { return b.cols, b.rows }
func (b *fakeBuffer) Resize(cols, rows int) {
	b.cols = cols
	b.rows = rows
}

func TestBuffer_InterfaceContract(t *testing.T) {
	t.Parallel()
	var b Buffer = newFake(80, 24)
	cols, rows := b.Size()
	if cols != 80 || rows != 24 {
		t.Errorf("Size = %d,%d want 80,24", cols, rows)
	}
	b.SetCell(5, 3, Cell{Ch: 'X', St: style.Style{Bold: true}})
	got := b.Cell(5, 3)
	if got.Ch != 'X' || !got.St.Bold {
		t.Errorf("Cell(5,3) = %+v want Ch='X' Bold=true", got)
	}
}

// TestBuffer_Resize (R2 claude MED-1, R3 codex MED-7): Resize updates Size().
func TestBuffer_Resize(t *testing.T) {
	t.Parallel()
	b := newFake(80, 24)
	b.Resize(120, 40)
	if cols, rows := b.Size(); cols != 120 || rows != 40 {
		t.Errorf("after Resize(120,40), Size = %d,%d want 120,40", cols, rows)
	}
}
