// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package buffer

import (
	"math"
	"testing"
)

func TestReset_BumpsGeneration(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(4, 3)
	before := g.generation
	g.Reset()
	if g.generation != before+1 {
		t.Errorf("generation after Reset = %d, want %d", g.generation, before+1)
	}
}

func TestReset_StaleCellReadsZero(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(4, 3)
	g.SetCell(1, 1, Cell{Ch: 'X'})
	if g.Cell(1, 1).Ch != 'X' {
		t.Fatal("pre-reset round-trip failed")
	}
	g.Reset()
	if got := g.Cell(1, 1); got != (Cell{}) {
		t.Errorf("post-Reset Cell(1,1) = %+v, want zero (stale generation)", got)
	}
}

func TestReset_SetCellAfterResetIsVisible(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(4, 3)
	g.SetCell(1, 1, Cell{Ch: 'X'})
	g.Reset()
	g.SetCell(2, 2, Cell{Ch: 'Y'})
	if got := g.Cell(2, 2); got.Ch != 'Y' {
		t.Errorf("post-Reset SetCell + read = %+v, want Ch='Y'", got)
	}
	// Earlier cell still stale.
	if got := g.Cell(1, 1); got != (Cell{}) {
		t.Errorf("Cell(1,1) post-Reset = %+v, want zero", got)
	}
}

// TestReset_Wraparound_Fallback (spec-1.2 R2 MED-1): force the
// generation to MaxUint64 so Reset triggers the overflow fallback;
// verify cells are explicitly cleared and generation restarts at 1.
func TestReset_Wraparound_Fallback(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(4, 3)
	g.SetCell(0, 0, Cell{Ch: 'A'})
	g.SetCell(2, 1, Cell{Ch: 'B'})
	g.generation = math.MaxUint64
	// Manually align cellGen with the forced generation so the pre-
	// reset reads are still "live"; this models a long-running
	// scheduler that bumped to MaxUint64 just before wraparound.
	for i := range g.cellGen {
		if g.cellGen[i] != 0 {
			g.cellGen[i] = math.MaxUint64
		}
	}
	g.Reset()
	if g.generation != 1 {
		t.Errorf("post-wrap generation = %d, want 1", g.generation)
	}
	// All cells must read zero (cellGen explicitly cleared to 0,
	// which differs from generation == 1).
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			if got := g.Cell(x, y); got != (Cell{}) {
				t.Errorf("post-wrap Cell(%d,%d) = %+v, want zero", x, y, got)
			}
		}
	}
	// And a fresh write after wrap is visible.
	g.SetCell(1, 1, Cell{Ch: 'Z'})
	if g.Cell(1, 1).Ch != 'Z' {
		t.Errorf("post-wrap SetCell read failed")
	}
}
