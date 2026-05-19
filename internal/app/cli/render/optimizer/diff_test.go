// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package optimizer

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
)

// helpers ------------------------------------------------------------

func mustGrid(t testing.TB, cols, rows int) *buffer.Grid {
	t.Helper()
	g, err := buffer.NewGrid(cols, rows)
	if err != nil {
		t.Fatalf("NewGrid(%d,%d): %v", cols, rows, err)
	}
	return g
}

func fillNarrow(g *buffer.Grid, ch rune) {
	cols, rows := g.Size()
	c := buffer.Cell{Ch: ch}
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			g.SetCell(x, y, c)
		}
	}
}

// table-driven cases -------------------------------------------------

func TestDiffEngine_NoChange_OneCell(t *testing.T) {
	t.Parallel()
	prev := mustGrid(t, 1, 1)
	next := mustGrid(t, 1, 1)
	patches := NewDiffEngine().Diff(prev, next)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches; got %+v", patches)
	}
}

func TestDiffEngine_NoChange_FilledIdentical(t *testing.T) {
	t.Parallel()
	prev := mustGrid(t, 10, 5)
	prev.SetCell(2, 2, buffer.Cell{Ch: 'A'})
	next := mustGrid(t, 10, 5)
	next.SetCell(2, 2, buffer.Cell{Ch: 'A'})
	patches := NewDiffEngine().Diff(prev, next)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches; got %+v", patches)
	}
}

func TestDiffEngine_SingleCellChange(t *testing.T) {
	t.Parallel()
	prev := mustGrid(t, 10, 5)
	prev.SetCell(2, 2, buffer.Cell{Ch: 'A'})
	next := mustGrid(t, 10, 5)
	next.SetCell(2, 2, buffer.Cell{Ch: 'B'})
	patches := NewDiffEngine().Diff(prev, next)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch; got %d: %+v", len(patches), patches)
	}
	want := Patch{Kind: PatchSetCell, X: 2, Y: 2, Cell: buffer.Cell{Ch: 'B'}}
	if patches[0] != want {
		t.Errorf("got %+v, want %+v", patches[0], want)
	}
}

func TestDiffEngine_MultiCellChange(t *testing.T) {
	t.Parallel()
	prev := mustGrid(t, 10, 5)
	fillNarrow(prev, 'A')
	next := mustGrid(t, 10, 5)
	fillNarrow(next, 'B')
	patches := NewDiffEngine().Diff(prev, next)
	if len(patches) != 50 {
		t.Errorf("expected 50 patches; got %d", len(patches))
	}
	for _, p := range patches {
		if p.Kind != PatchSetCell || p.Cell.Ch != 'B' {
			t.Errorf("unexpected patch %+v", p)
			break
		}
	}
}

func TestDiffEngine_WideRuneChange(t *testing.T) {
	t.Parallel()
	prev := mustGrid(t, 10, 5)
	prev.SetCell(2, 2, buffer.Cell{Ch: 'A'})
	next := mustGrid(t, 10, 5)
	// SetCell auto-writes continuation at (3,2) via spec-1.2 R2 D1.
	next.SetCell(2, 2, buffer.Cell{Ch: '中'})
	patches := NewDiffEngine().Diff(prev, next)
	// Expect 1 main-cell patch; the continuation at (3,2) is skipped via
	// buffer.IsContinuation(next.Cell(3,2)) == true (spec-1.2 R2 D1
	// consumer contract). Also (3,2) of prev was Cell{} which equals
	// nothing meaningful in this path; the IsContinuation skip happens
	// before the prev/next compare.
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch (main only); got %d: %+v", len(patches), patches)
	}
	got := patches[0]
	if got.Kind != PatchSetCell || got.X != 2 || got.Y != 2 || got.Cell.Ch != '中' {
		t.Errorf("got %+v, want PatchSetCell{X:2,Y:2,Cell:中}", got)
	}
	// Sanity-assert IsContinuation API on next's (3,2) so the test
	// references the SSOT explicitly (spec-1.3 R2 codex R2 LOW-2).
	if !buffer.IsContinuation(next.Cell(3, 2)) {
		t.Errorf("next.Cell(3,2) should be continuation; got %+v", next.Cell(3, 2))
	}
}

func TestDiffEngine_ContinuationSkip(t *testing.T) {
	t.Parallel()
	prev := mustGrid(t, 10, 5)
	prev.SetCell(2, 2, buffer.Cell{Ch: '中'})
	next := mustGrid(t, 10, 5)
	next.SetCell(2, 2, buffer.Cell{Ch: '中'})
	patches := NewDiffEngine().Diff(prev, next)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches; got %+v", patches)
	}
	// IsContinuation API both sides (spec-1.3 R2 codex R2 LOW-2).
	if !buffer.IsContinuation(prev.Cell(3, 2)) {
		t.Errorf("prev.Cell(3,2) should be continuation")
	}
	if !buffer.IsContinuation(next.Cell(3, 2)) {
		t.Errorf("next.Cell(3,2) should be continuation")
	}
}

// TestDiffEngine_ContinuationToNarrow — spec-1.3 R2 claude HIGH-3 docs
// gap test: prev has wide rune at (2,2)+continuation at (3,2); next
// writes narrow 'A' at (2,2). spec-1.2 R3 wide-overlap auto-clear
// triggers in next.SetCell so next.Cell(3,2) returns Cell{} (NOT
// WideContinuation). IsContinuation(next.Cell(3,2)) is therefore false,
// the diff compares prev WideContinuation vs next Cell{} (not equal)
// and emits PatchSetCell{X:3,Y:2,Cell{}} — Driver clears that column.
func TestDiffEngine_ContinuationToNarrow(t *testing.T) {
	t.Parallel()
	prev := mustGrid(t, 10, 5)
	prev.SetCell(2, 2, buffer.Cell{Ch: '中'})
	next := mustGrid(t, 10, 5)
	next.SetCell(2, 2, buffer.Cell{Ch: 'A'})

	// Verify the spec-1.2 R3 auto-clear precondition: next (3,2) is
	// NOT a continuation cell (spec-1.3 R2 codex R2 LOW-2 + claude HIGH-3).
	if buffer.IsContinuation(next.Cell(3, 2)) {
		t.Fatalf("next.Cell(3,2) unexpectedly IS continuation; spec-1.2 R3 auto-clear broken?")
	}
	if !buffer.IsContinuation(prev.Cell(3, 2)) {
		t.Fatalf("prev.Cell(3,2) should be continuation")
	}

	patches := NewDiffEngine().Diff(prev, next)
	if len(patches) != 2 {
		t.Fatalf("expected 2 patches (main + clear); got %d: %+v", len(patches), patches)
	}
	if patches[0] != (Patch{Kind: PatchSetCell, X: 2, Y: 2, Cell: buffer.Cell{Ch: 'A'}}) {
		t.Errorf("patches[0] = %+v; want PatchSetCell{X:2,Y:2,A}", patches[0])
	}
	if patches[1] != (Patch{Kind: PatchSetCell, X: 3, Y: 2, Cell: buffer.Cell{}}) {
		t.Errorf("patches[1] = %+v; want PatchSetCell{X:3,Y:2,Cell{}}", patches[1])
	}
}

func TestDiffEngine_Resize_GrowsViewport(t *testing.T) {
	t.Parallel()
	prev := mustGrid(t, 10, 5)
	fillNarrow(prev, 'A')
	next := mustGrid(t, 20, 10)
	fillNarrow(next, 'A') // setup explicit fill (spec-1.3 R2 codex R3 LOW-1)
	patches := NewDiffEngine().Diff(prev, next)
	// Expect: PatchResize{20,10} + 200 × PatchSetCell{'A'}.
	if len(patches) != 201 {
		t.Fatalf("expected 201 patches (1 resize + 200 cells); got %d", len(patches))
	}
	if patches[0].Kind != PatchResize || patches[0].NewCols != 20 || patches[0].NewRows != 10 {
		t.Errorf("patches[0] = %+v; want PatchResize{20,10}", patches[0])
	}
	for _, p := range patches[1:] {
		if p.Kind != PatchSetCell || p.Cell.Ch != 'A' {
			t.Errorf("unexpected patch %+v", p)
			break
		}
	}
}

func TestDiffEngine_Resize_ShrinksViewport(t *testing.T) {
	t.Parallel()
	prev := mustGrid(t, 20, 10)
	fillNarrow(prev, 'A')
	next := mustGrid(t, 10, 5)
	fillNarrow(next, 'A')
	patches := NewDiffEngine().Diff(prev, next)
	if len(patches) != 51 {
		t.Fatalf("expected 51 patches (1 resize + 50 cells); got %d", len(patches))
	}
	if patches[0].Kind != PatchResize || patches[0].NewCols != 10 || patches[0].NewRows != 5 {
		t.Errorf("patches[0] = %+v; want PatchResize{10,5}", patches[0])
	}
}

func TestDiffEngine_FirstFrame_NilPrev(t *testing.T) {
	t.Parallel()
	next := mustGrid(t, 10, 5)
	next.SetCell(2, 2, buffer.Cell{Ch: 'A'})
	patches := NewDiffEngine().Diff(nil, next)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch (fullRedraw skips zero cells); got %d", len(patches))
	}
	want := Patch{Kind: PatchSetCell, X: 2, Y: 2, Cell: buffer.Cell{Ch: 'A'}}
	if patches[0] != want {
		t.Errorf("got %+v, want %+v", patches[0], want)
	}
}

// TestDiffEngine_FirstFrame_NilPrev_EmptyNext — spec-1.3 R2 4-path
// CRIT-1 fix: G1 clean-surface precondition means the Driver has
// already established a blank surface; a fresh-alloc all-zero next
// produces zero patches (NOT 50 PatchSetCell{Cell{}}).
func TestDiffEngine_FirstFrame_NilPrev_EmptyNext(t *testing.T) {
	t.Parallel()
	next := mustGrid(t, 10, 5)
	patches := NewDiffEngine().Diff(nil, next)
	if len(patches) != 0 {
		t.Errorf("expected 0 patches (G1 clean-surface fullRedraw skip Cell{}); got %d: %+v",
			len(patches), patches)
	}
}

// TestDiffEngine_FullClear_PrevNonZero — normal diff path (prev != nil,
// !resized): prev all 'A', next all Cell{} → emit 50 clear patches.
// This is the counterpart to the fullRedraw skip in NilPrev_EmptyNext;
// the difference is the diff path is selected by prev being non-nil.
func TestDiffEngine_FullClear_PrevNonZero(t *testing.T) {
	t.Parallel()
	prev := mustGrid(t, 10, 5)
	fillNarrow(prev, 'A')
	next := mustGrid(t, 10, 5) // all Cell{}
	patches := NewDiffEngine().Diff(prev, next)
	if len(patches) != 50 {
		t.Fatalf("expected 50 clear patches; got %d", len(patches))
	}
	for _, p := range patches {
		if p.Kind != PatchSetCell || p.Cell != (buffer.Cell{}) {
			t.Errorf("unexpected patch %+v; want PatchSetCell{Cell{}}", p)
			break
		}
	}
}

func TestDiffEngine_StyleOnlyChange(t *testing.T) {
	t.Parallel()
	red := style.Style{FG: style.RGB(255, 0, 0)}
	blue := style.Style{FG: style.RGB(0, 0, 255)}
	prev := mustGrid(t, 10, 5)
	prev.SetCell(2, 2, buffer.Cell{Ch: 'A', St: red})
	next := mustGrid(t, 10, 5)
	next.SetCell(2, 2, buffer.Cell{Ch: 'A', St: blue})
	patches := NewDiffEngine().Diff(prev, next)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch (style-only change); got %d", len(patches))
	}
	if patches[0].Cell.St != blue {
		t.Errorf("patch style = %+v; want blue", patches[0].Cell.St)
	}
}

func TestDiffEngine_DifferentRuneSameStyle(t *testing.T) {
	t.Parallel()
	red := style.Style{FG: style.RGB(255, 0, 0)}
	prev := mustGrid(t, 10, 5)
	prev.SetCell(2, 2, buffer.Cell{Ch: 'A', St: red})
	next := mustGrid(t, 10, 5)
	next.SetCell(2, 2, buffer.Cell{Ch: 'B', St: red})
	patches := NewDiffEngine().Diff(prev, next)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch (rune change); got %d", len(patches))
	}
	want := Patch{Kind: PatchSetCell, X: 2, Y: 2, Cell: buffer.Cell{Ch: 'B', St: red}}
	if patches[0] != want {
		t.Errorf("got %+v, want %+v", patches[0], want)
	}
}

func TestDiffEngine_LastColumnWideRune(t *testing.T) {
	t.Parallel()
	prev := mustGrid(t, 10, 5)
	next := mustGrid(t, 10, 5)
	// Wide rune at last column; spec-1.2 silent no-op for continuation
	// at (10, 2). main cell at (9, 2) is written.
	next.SetCell(9, 2, buffer.Cell{Ch: '中'})
	patches := NewDiffEngine().Diff(prev, next)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch (main only, no continuation); got %d", len(patches))
	}
	if patches[0].X != 9 || patches[0].Y != 2 || patches[0].Cell.Ch != '中' {
		t.Errorf("got %+v", patches[0])
	}
	// (10, 2) is out of bounds — buffer.Cell read returns Cell{} not
	// continuation; IsContinuation API doesn't fire on the missing
	// last column (spec-1.3 R2 codex R2 LOW-2).
	if buffer.IsContinuation(next.Cell(10, 2)) {
		t.Errorf("(10,2) OOB; should NOT be continuation")
	}
}

// TestDiffEngine_Concurrent — spec-1.3 R2 Q8 ★A + codex R1 MED-4 pin:
// 16 goroutines × 200 iterations each, sharing one *DiffEngine, with
// independent prev/next Grids per goroutine. Validates the zero-state
// shared safety claim under the race detector.
func TestDiffEngine_Concurrent(t *testing.T) {
	t.Parallel()
	const goroutines = 16
	const iterations = 200
	engine := NewDiffEngine()
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var errs atomic.Int64
	for gi := 0; gi < goroutines; gi++ {
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				prev := mustGrid(t, 10, 5)
				prev.SetCell(seed%10, i%5, buffer.Cell{Ch: 'A'})
				next := mustGrid(t, 10, 5)
				next.SetCell(seed%10, i%5, buffer.Cell{Ch: 'B'})
				patches := engine.Diff(prev, next)
				if len(patches) != 1 || patches[0].Cell.Ch != 'B' {
					errs.Add(1)
					return
				}
			}
		}(gi)
	}
	wg.Wait()
	if errs.Load() != 0 {
		t.Errorf("concurrent Diff saw %d errors", errs.Load())
	}
}
