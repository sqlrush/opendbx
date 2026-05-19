// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package buffer

import (
	"errors"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
)

// --- NewGrid -----------------------------------------------------------

func TestNewGrid_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		cols, rows int
	}{
		{"1x1", 1, 1},
		{"80x24", 80, 24},
		{"200x60", 200, 60},
		{"500x200", 500, 200},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			g, err := NewGrid(c.cols, c.rows)
			if err != nil {
				t.Fatalf("NewGrid(%d,%d) err = %v, want nil", c.cols, c.rows, err)
			}
			if g == nil {
				t.Fatal("NewGrid returned nil grid without error")
			}
			cols, rows := g.Size()
			if cols != c.cols || rows != c.rows {
				t.Errorf("Size = %d,%d, want %d,%d", cols, rows, c.cols, c.rows)
			}
			if g.generation != 1 {
				t.Errorf("fresh generation = %d, want 1 (R2 MED-1)", g.generation)
			}
		})
	}
}

func TestNewGrid_Invalid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		cols, rows int
	}{
		{"zero_cols", 0, 24},
		{"zero_rows", 80, 0},
		{"zero_both", 0, 0},
		{"negative_cols", -1, 24},
		{"negative_rows", 80, -5},
		{"both_negative", -1, -1},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			g, err := NewGrid(c.cols, c.rows)
			if g != nil {
				t.Errorf("NewGrid(%d,%d) grid = %p, want nil", c.cols, c.rows, g)
			}
			if !errors.Is(err, ErrInvalidDimension) {
				t.Errorf("NewGrid(%d,%d) err = %v, want ErrInvalidDimension", c.cols, c.rows, err)
			}
		})
	}
}

func TestNewGrid_Overflow(t *testing.T) {
	t.Parallel()
	// cols × rows > MaxInt32 — pick numbers comfortably over the bound.
	g, err := NewGrid(70_000, 70_000)
	if g != nil || !errors.Is(err, ErrInvalidDimension) {
		t.Errorf("NewGrid(70000,70000) = (%p, %v), want (nil, ErrInvalidDimension)", g, err)
	}
}

// --- Cell / SetCell ---------------------------------------------------

func TestGrid_CellSetCell_RoundTrip(t *testing.T) {
	t.Parallel()
	g, err := NewGrid(10, 5)
	if err != nil {
		t.Fatal(err)
	}
	c := Cell{Ch: 'A', St: style.Style{Bold: true}}
	g.SetCell(3, 2, c)
	got := g.Cell(3, 2)
	if got != c {
		t.Errorf("Cell(3,2) = %+v, want %+v", got, c)
	}
}

func TestGrid_Cell_OutOfBounds_ReturnsZero(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(10, 5)
	cases := [][2]int{{-1, 0}, {0, -1}, {10, 0}, {0, 5}, {100, 100}, {-100, -100}}
	for _, p := range cases {
		got := g.Cell(p[0], p[1])
		if got != (Cell{}) {
			t.Errorf("Cell(%d,%d) = %+v, want zero Cell{}", p[0], p[1], got)
		}
	}
}

func TestGrid_SetCell_OutOfBounds_NoOp(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(10, 5)
	// Write OOB then read in-bounds; the in-bounds cell must remain
	// the zero cell (post-Reset semantics).
	g.SetCell(100, 100, Cell{Ch: 'X'})
	g.SetCell(-1, -1, Cell{Ch: 'Y'})
	g.SetCell(10, 0, Cell{Ch: 'Z'})
	g.SetCell(0, 5, Cell{Ch: 'W'})
	for y := 0; y < 5; y++ {
		for x := 0; x < 10; x++ {
			if got := g.Cell(x, y); got != (Cell{}) {
				t.Errorf("Cell(%d,%d) = %+v after OOB writes, want zero", x, y, got)
			}
		}
	}
}

func TestGrid_Size(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(120, 40)
	cols, rows := g.Size()
	if cols != 120 || rows != 40 {
		t.Errorf("Size = %d,%d, want 120,40", cols, rows)
	}
}

// --- Resize -----------------------------------------------------------

func TestGrid_Resize_Grow_PreservesCells(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(10, 5)
	g.SetCell(0, 0, Cell{Ch: 'A'})
	g.SetCell(9, 4, Cell{Ch: 'Z'})
	g.SetCell(5, 2, Cell{Ch: 'M'})
	g.Resize(20, 10)
	cols, rows := g.Size()
	if cols != 20 || rows != 10 {
		t.Fatalf("after Resize(20,10), Size = %d,%d", cols, rows)
	}
	if g.Cell(0, 0).Ch != 'A' {
		t.Errorf("Cell(0,0).Ch = %q, want A", g.Cell(0, 0).Ch)
	}
	if g.Cell(9, 4).Ch != 'Z' {
		t.Errorf("Cell(9,4).Ch = %q, want Z", g.Cell(9, 4).Ch)
	}
	if g.Cell(5, 2).Ch != 'M' {
		t.Errorf("Cell(5,2).Ch = %q, want M", g.Cell(5, 2).Ch)
	}
	// New area must read as zero.
	if got := g.Cell(15, 8); got != (Cell{}) {
		t.Errorf("Cell(15,8) post-grow = %+v, want zero", got)
	}
}

func TestGrid_Resize_Shrink_DropsOutside(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(10, 5)
	g.SetCell(0, 0, Cell{Ch: 'A'})
	g.SetCell(8, 4, Cell{Ch: 'X'}) // will be dropped
	g.SetCell(2, 1, Cell{Ch: 'B'})
	g.Resize(5, 3)
	if g.Cell(0, 0).Ch != 'A' {
		t.Errorf("Cell(0,0) lost on shrink")
	}
	if g.Cell(2, 1).Ch != 'B' {
		t.Errorf("Cell(2,1) lost on shrink")
	}
	// (8,4) is now out-of-bounds → zero.
	if got := g.Cell(8, 4); got != (Cell{}) {
		t.Errorf("Cell(8,4) post-shrink = %+v, want zero (OOB)", got)
	}
}

// TestGrid_Resize_ShrinkRowsOnly_InPlace exercises the fast-path
// reslice branch (same cols, rows shrink, no reallocation). spec-1.2
// R3 (go-reviewer LOW-1): the general per-row copy path is well
// covered, but the cheaper reslice branch deserves its own assertion
// that surviving rows keep their (x, y) coordinates AND that the
// backing capacity is unchanged (so Release routes to the original
// bucket per the cap-not-dim contract).
func TestGrid_Resize_ShrinkRowsOnly_InPlace(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(10, 5)
	g.SetCell(0, 0, Cell{Ch: 'A'})
	g.SetCell(9, 0, Cell{Ch: 'B'})
	g.SetCell(0, 2, Cell{Ch: 'C'})
	g.SetCell(9, 4, Cell{Ch: 'D'}) // will be dropped
	wantCap := cap(g.cells)
	g.Resize(10, 3) // same cols, rows shrink → fast-path reslice
	cols, rows := g.Size()
	if cols != 10 || rows != 3 {
		t.Errorf("Resize size = %d,%d, want 10,3", cols, rows)
	}
	if cap(g.cells) != wantCap {
		t.Errorf("backing cap changed under fast-path reslice: %d → %d", wantCap, cap(g.cells))
	}
	if g.Cell(0, 0).Ch != 'A' || g.Cell(9, 0).Ch != 'B' || g.Cell(0, 2).Ch != 'C' {
		t.Error("surviving cells did not retain (x,y) after fast-path shrink")
	}
	if got := g.Cell(9, 4); got != (Cell{}) {
		t.Errorf("Cell(9,4) post-shrink = %+v, want zero (OOB)", got)
	}
}

func TestGrid_Resize_Same_NoOp(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(10, 5)
	g.SetCell(3, 3, Cell{Ch: 'Q'})
	g.Resize(10, 5)
	if g.Cell(3, 3).Ch != 'Q' {
		t.Errorf("Cell(3,3) lost on same-dim Resize")
	}
}

func TestGrid_Resize_PerRowCopy_ColsChange(t *testing.T) {
	t.Parallel()
	// Fill a 4x3 grid such that (x,y) cells are distinguishable.
	g, _ := NewGrid(4, 3)
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			g.SetCell(x, y, Cell{Ch: rune('0' + y*4 + x)})
		}
	}
	g.Resize(6, 3)
	// (0,0)='0' (1,0)='1' (2,0)='2' (3,0)='3' must keep their X.
	// Critically: (0,1) must be '4' not '3' — if reslice happened
	// instead of per-row copy, (0,1) would shift to whatever cell
	// 4 happened to be in the OLD layout.
	if g.Cell(0, 1).Ch != '4' {
		t.Errorf("Cell(0,1) = %q, want '4' — Resize(cols change) must per-row copy",
			g.Cell(0, 1).Ch)
	}
	if g.Cell(3, 1).Ch != '7' {
		t.Errorf("Cell(3,1) = %q, want '7'", g.Cell(3, 1).Ch)
	}
	if g.Cell(3, 2).Ch != ';' {
		t.Errorf("Cell(3,2) = %q, want ';' (rune 8+3+0x30=';')", g.Cell(3, 2).Ch)
	}
}

func TestGrid_Resize_CellGenPreserved(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(4, 3)
	g.SetCell(1, 1, Cell{Ch: 'X'})
	// Bump generation a few times, then SetCell again so cellGen[1,1]
	// holds the latest generation value.
	for i := 0; i < 3; i++ {
		g.Reset()
	}
	g.SetCell(1, 1, Cell{Ch: 'Y'})
	wantGen := g.generation
	g.Resize(6, 5)
	// After Resize, the (1,1) cell must still read 'Y' — i.e. its
	// cellGen stamp was preserved verbatim, not reset to zero.
	if got := g.Cell(1, 1); got.Ch != 'Y' {
		t.Errorf("post-Resize Cell(1,1) = %+v, want Ch='Y' (cellGen verbatim copy)", got)
	}
	if g.generation != wantGen {
		t.Errorf("generation changed across Resize: %d → %d", wantGen, g.generation)
	}
}

func TestGrid_Resize_Invalid_Ignored(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(10, 5)
	g.SetCell(0, 0, Cell{Ch: 'A'})
	g.Resize(-1, 5)
	g.Resize(10, 0)
	g.Resize(0, 0)         // spec-1.2 R3 LOW: explicit (0,0) edge case
	g.Resize(70000, 70000) // overflow
	cols, rows := g.Size()
	if cols != 10 || rows != 5 {
		t.Errorf("invalid Resize altered dims: %d,%d", cols, rows)
	}
	if g.Cell(0, 0).Ch != 'A' {
		t.Errorf("invalid Resize wiped cells")
	}
}

// --- Buffer interface conformance -------------------------------------

func TestGrid_ImplementsBuffer(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(5, 5)
	var b Buffer = g
	b.SetCell(1, 1, Cell{Ch: 'H'})
	if b.Cell(1, 1).Ch != 'H' {
		t.Errorf("Buffer interface SetCell/Cell round-trip failed")
	}
	cols, rows := b.Size()
	if cols != 5 || rows != 5 {
		t.Errorf("Buffer.Size = %d,%d, want 5,5", cols, rows)
	}
	b.Resize(8, 8)
	if cols2, rows2 := b.Size(); cols2 != 8 || rows2 != 8 {
		t.Errorf("Buffer.Resize then Size = %d,%d, want 8,8", cols2, rows2)
	}
}
