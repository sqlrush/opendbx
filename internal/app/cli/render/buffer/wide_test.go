// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package buffer

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
)

// --- IsContinuation predicate (spec-1.2 D-4 / R2 D1 A2) ---------------

func TestIsContinuation_True_OnWideContinuation(t *testing.T) {
	t.Parallel()
	if !IsContinuation(Cell{Ch: WideContinuation}) {
		t.Errorf("IsContinuation(Cell{Ch: WideContinuation}) = false, want true")
	}
}

func TestIsContinuation_False_OnEmptyCell(t *testing.T) {
	t.Parallel()
	if IsContinuation(Cell{}) {
		t.Errorf("IsContinuation(Cell{}) = true, want false (R2 D1 — empty NOT same as continuation)")
	}
	// Explicit Ch=0 (e.g. NUL written deliberately) must also be NOT-continuation.
	if IsContinuation(Cell{Ch: 0, St: style.Style{Bold: true}}) {
		t.Errorf("IsContinuation(Cell{Ch: 0}) = true, want false")
	}
}

func TestIsContinuation_False_OnNarrowRune(t *testing.T) {
	t.Parallel()
	cases := []rune{'a', 'A', ' ', '中', '文', 0x200B /* ZWSP */}
	for _, r := range cases {
		if IsContinuation(Cell{Ch: r}) {
			t.Errorf("IsContinuation(Cell{Ch: %q}) = true, want false", r)
		}
	}
}

func TestWideContinuation_IsNegativeOne(t *testing.T) {
	t.Parallel()
	if WideContinuation != -1 {
		t.Errorf("WideContinuation = %d, want -1 (spec-1.2 D-4)", WideContinuation)
	}
}

// --- SetCell wide-rune auto-detect ------------------------------------

func TestSetCell_WideRune_WritesMainAndContinuation(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(10, 3)
	wide := Cell{Ch: '中', St: style.Style{Bold: true}}
	g.SetCell(2, 1, wide)
	// Main cell at (2, 1).
	if got := g.Cell(2, 1); got.Ch != '中' || !got.St.Bold {
		t.Errorf("Cell(2,1) = %+v, want main wide rune", got)
	}
	// Continuation at (3, 1).
	cont := g.Cell(3, 1)
	if !IsContinuation(cont) {
		t.Errorf("Cell(3,1) = %+v, want IsContinuation=true", cont)
	}
	if !cont.St.Bold {
		t.Errorf("continuation cell style = %+v, want Bold inherited", cont.St)
	}
}

func TestSetCell_WideRune_LastColumn_NoContinuation(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(4, 2)
	wide := Cell{Ch: '中'}
	g.SetCell(3, 0, wide) // x == cols-1 → continuation would be OOB
	if got := g.Cell(3, 0); got.Ch != '中' {
		t.Errorf("Cell(3,0) = %+v, want main cell to survive", got)
	}
	// No continuation written (silent — main still placed).
	// Confirm (0, 1) is still zero.
	if got := g.Cell(0, 1); got != (Cell{}) {
		t.Errorf("Cell(0,1) = %+v, want zero (no continuation leak)", got)
	}
}

func TestSetCell_NarrowRune_NoContinuation(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(5, 1)
	g.SetCell(2, 0, Cell{Ch: 'a'})
	// (3, 0) must remain empty — no spurious continuation.
	if got := g.Cell(3, 0); got != (Cell{}) {
		t.Errorf("Cell(3,0) after narrow SetCell = %+v, want zero", got)
	}
}

// TestSetCell_WideOverwrite_OverwritesContinuation pins the contract
// that overwriting a wide rune with another wide rune updates both
// the main and continuation cells coherently.
func TestSetCell_WideOverwrite_OverwritesContinuation(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(6, 1)
	g.SetCell(1, 0, Cell{Ch: '中'})
	g.SetCell(1, 0, Cell{Ch: '文', St: style.Style{Italic: true}})
	if g.Cell(1, 0).Ch != '文' {
		t.Errorf("Cell(1,0) = %q, want '文' after overwrite", g.Cell(1, 0).Ch)
	}
	cont := g.Cell(2, 0)
	if !IsContinuation(cont) || !cont.St.Italic {
		t.Errorf("Cell(2,0) = %+v, want continuation w/ Italic inherited", cont)
	}
}

// TestSetCell_WideThenNarrow_LeavesStaleContinuation documents the
// current contract: writing a narrow rune over a previous wide rune
// main cell does NOT auto-clear the next column's continuation. The
// caller (spec-1.3 optimizer / spec-1.7 block composer) is expected
// to clear that cell explicitly when the layout changes.
func TestSetCell_WideThenNarrow_LeavesStaleContinuation(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(4, 1)
	g.SetCell(0, 0, Cell{Ch: '中'})
	// Overwriting the main cell with a narrow rune leaves the
	// continuation cell at (1, 0) unchanged.
	g.SetCell(0, 0, Cell{Ch: 'a'})
	if g.Cell(0, 0).Ch != 'a' {
		t.Errorf("Cell(0,0) = %q, want 'a'", g.Cell(0, 0).Ch)
	}
	if !IsContinuation(g.Cell(1, 0)) {
		t.Errorf("Cell(1,0) = %+v, want continuation (caller responsibility to clear)", g.Cell(1, 0))
	}
}

func TestSetCell_WideRune_ZeroStyle(t *testing.T) {
	t.Parallel()
	g, _ := NewGrid(4, 1)
	g.SetCell(0, 0, Cell{Ch: '中'})
	cont := g.Cell(1, 0)
	if !IsContinuation(cont) {
		t.Errorf("Cell(1,0) = %+v, want continuation", cont)
	}
	if (cont.St != style.Style{}) {
		t.Errorf("Cell(1,0).St = %+v, want zero", cont.St)
	}
}
