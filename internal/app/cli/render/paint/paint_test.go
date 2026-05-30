// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package paint

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// mkGrid returns a fresh dst grid with the given dimensions; t.Fatal on err.
func mkGrid(t *testing.T, cols, rows int) *buffer.Grid {
	t.Helper()
	g, err := buffer.NewGrid(cols, rows)
	if err != nil {
		t.Fatalf("NewGrid(%d, %d): %v", cols, rows, err)
	}
	return g
}

// ============================================================
// Blit — top-left bounded copy
// ============================================================

func TestBlit_PreservesWideRunes(t *testing.T) {
	t.Parallel()
	src := mkGrid(t, 8, 1)
	src.SetCell(0, 0, buffer.Cell{Ch: '你'})
	src.SetCell(2, 0, buffer.Cell{Ch: '好'})

	dst := mkGrid(t, 8, 1)
	Blit(dst, src, 8, 1)

	if got := dst.Cell(0, 0).Ch; got != '你' {
		t.Errorf("dst(0,0)=%q; want 你 (wide-main preserved)", got)
	}
	if !buffer.IsContinuation(dst.Cell(1, 0)) {
		t.Errorf("dst(1,0) should be continuation; got %+v", dst.Cell(1, 0))
	}
	if got := dst.Cell(2, 0).Ch; got != '好' {
		t.Errorf("dst(2,0)=%q; want 好", got)
	}
	if !buffer.IsContinuation(dst.Cell(3, 0)) {
		t.Errorf("dst(3,0) should be continuation; got %+v", dst.Cell(3, 0))
	}
}

func TestBlit_NilSrc(t *testing.T) {
	t.Parallel()
	dst := mkGrid(t, 4, 1)
	dst.SetCell(0, 0, buffer.Cell{Ch: 'A'}) // sentinel
	Blit(dst, nil, 4, 1)
	if dst.Cell(0, 0).Ch != 'A' {
		t.Errorf("nil src must not mutate dst")
	}
}

func TestBlit_NilDst(t *testing.T) {
	t.Parallel()
	src := mkGrid(t, 4, 1)
	// Must not panic.
	Blit(nil, src, 4, 1)
}

func TestBlit_ZeroDimsNoop(t *testing.T) {
	t.Parallel()
	src := mkGrid(t, 4, 4)
	src.SetCell(0, 0, buffer.Cell{Ch: 'X'})
	dst := mkGrid(t, 4, 4)
	dst.SetCell(0, 0, buffer.Cell{Ch: 'A'})
	Blit(dst, src, 0, 4)
	Blit(dst, src, 4, 0)
	Blit(dst, src, -1, 4)
	Blit(dst, src, 4, -1)
	if dst.Cell(0, 0).Ch != 'A' {
		t.Errorf("zero/negative dims must be no-op; got %q", dst.Cell(0, 0).Ch)
	}
}

func TestBlit_ClipBoundsCallerSupplied(t *testing.T) {
	// Caller-supplied cols/rows < src.Size() must HARD clip (mirrors the
	// pre-paint scrollback/cache.go copyCells contract).
	t.Parallel()
	src := mkGrid(t, 6, 3)
	for x := 0; x < 6; x++ {
		for y := 0; y < 3; y++ {
			src.SetCell(x, y, buffer.Cell{Ch: 'X'})
		}
	}
	dst := mkGrid(t, 6, 3)
	Blit(dst, src, 2, 2) // clip to top-left 2×2

	for y := 0; y < 3; y++ {
		for x := 0; x < 6; x++ {
			got := dst.Cell(x, y).Ch
			inClip := x < 2 && y < 2
			wantSet := inClip && got == 'X'
			wantUnset := !inClip && got == 0
			if !wantSet && !wantUnset {
				t.Errorf("dst(%d,%d)=%q; clip=%v wanted set=%v unset=%v",
					x, y, got, inClip, inClip, !inClip)
			}
		}
	}
}

func TestBlit_ClipBoundsSrcSmaller(t *testing.T) {
	// cols > src.Size() must intersect down to src extent — not over-read.
	t.Parallel()
	src := mkGrid(t, 2, 2)
	src.SetCell(0, 0, buffer.Cell{Ch: 'A'})
	src.SetCell(1, 1, buffer.Cell{Ch: 'B'})
	dst := mkGrid(t, 5, 5)
	Blit(dst, src, 5, 5)
	if dst.Cell(0, 0).Ch != 'A' || dst.Cell(1, 1).Ch != 'B' {
		t.Errorf("src extent not honored: dst(0,0)=%q dst(1,1)=%q",
			dst.Cell(0, 0).Ch, dst.Cell(1, 1).Ch)
	}
	if dst.Cell(3, 3).Ch != 0 {
		t.Errorf("dst(3,3) should remain zero (src 2×2); got %q", dst.Cell(3, 3).Ch)
	}
}

// ============================================================
// BlitAt — offset + crop
// ============================================================

func TestBlitAt_OffsetPositive(t *testing.T) {
	t.Parallel()
	src := mkGrid(t, 2, 1)
	src.SetCell(0, 0, buffer.Cell{Ch: '你'})
	dst := mkGrid(t, 10, 3)
	BlitAt(dst, src, 3, 2)
	if got := dst.Cell(3, 2).Ch; got != '你' {
		t.Errorf("dst(3,2)=%q; want 你 (offset paint)", got)
	}
	if !buffer.IsContinuation(dst.Cell(4, 2)) {
		t.Errorf("dst(4,2) should be continuation")
	}
}

func TestBlitAt_NegativeOffsetCrops(t *testing.T) {
	// Negative offset must crop silently — matches existing paintBufferAt
	// behavior used by virtual.composeInto and llmapp/model paintBufferAt.
	t.Parallel()
	src := mkGrid(t, 4, 2)
	for x := 0; x < 4; x++ {
		for y := 0; y < 2; y++ {
			src.SetCell(x, y, buffer.Cell{Ch: 'X'})
		}
	}
	dst := mkGrid(t, 4, 2)
	BlitAt(dst, src, -1, -1) // crop top-left away
	// dst(0..2, 0..0) should hold X (from src(1..3, 1)).
	if dst.Cell(0, 0).Ch != 'X' {
		t.Errorf("dst(0,0) after negative offset = %q; want X", dst.Cell(0, 0).Ch)
	}
}

func TestBlitAt_OverflowCrops(t *testing.T) {
	t.Parallel()
	src := mkGrid(t, 4, 1)
	for x := 0; x < 4; x++ {
		src.SetCell(x, 0, buffer.Cell{Ch: 'X'})
	}
	dst := mkGrid(t, 3, 1)
	BlitAt(dst, src, 1, 0) // src(0..2) → dst(1..3), but dst is 3 wide
	if dst.Cell(1, 0).Ch != 'X' || dst.Cell(2, 0).Ch != 'X' {
		t.Errorf("dst(1,0)=%q dst(2,0)=%q; want X X", dst.Cell(1, 0).Ch, dst.Cell(2, 0).Ch)
	}
}

func TestBlitAt_NilSrc(t *testing.T) {
	t.Parallel()
	dst := mkGrid(t, 4, 1)
	dst.SetCell(0, 0, buffer.Cell{Ch: 'A'})
	BlitAt(dst, nil, 1, 0)
	if dst.Cell(0, 0).Ch != 'A' {
		t.Errorf("nil src must not mutate dst")
	}
}

func TestBlitAt_NilDst(t *testing.T) {
	t.Parallel()
	src := mkGrid(t, 4, 1)
	// Must not panic.
	BlitAt(nil, src, 0, 0)
}

// TestBlitAt_NoMojibakeOnMixedCJKAscii is the spec-1.20.2 D-6 invariant
// guard captured at unit level: a row of mixed wide CJK + ASCII MUST
// round-trip without scrambling — proves the helper closes the entire
// class of bug found 5/28 (CJK chars getting split by continuation
// over-writes).
func TestBlitAt_NoMojibakeOnMixedCJKAscii(t *testing.T) {
	t.Parallel()
	src := mkGrid(t, 10, 1)
	src.SetCell(0, 0, buffer.Cell{Ch: '你'})
	src.SetCell(2, 0, buffer.Cell{Ch: 'A'})
	src.SetCell(3, 0, buffer.Cell{Ch: '好'})
	src.SetCell(5, 0, buffer.Cell{Ch: 'B'})
	dst := mkGrid(t, 10, 1)
	BlitAt(dst, src, 0, 0)

	want := []rune{'你', 0, 'A', '好', 0, 'B'} // 0 = continuation cell
	for i, w := range want {
		got := dst.Cell(i, 0).Ch
		if w == 0 {
			if !buffer.IsContinuation(dst.Cell(i, 0)) {
				t.Errorf("dst(%d,0) should be continuation; got %+v", i, dst.Cell(i, 0))
			}
		} else if got != w {
			t.Errorf("dst(%d,0)=%q; want %q (CJK+ASCII mix mojibake guard)", i, got, w)
		}
	}
}
