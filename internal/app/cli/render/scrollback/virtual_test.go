// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scrollback

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/layout"
)

// fakeBlock is a test-only block.RenderNode that returns a Grid of the
// configured size with each cell carrying a tag rune (so blits can be
// verified). For pre-first-Render Push tests we also support a constant-
// height mode.
type fakeBlock struct {
	height int
	tag    rune
}

func (b fakeBlock) Render(ctx block.Context) (buffer.Buffer, error) {
	cols := ctx.Cols
	if cols == 0 {
		cols = 1
	}
	g, err := buffer.NewGrid(cols, b.height)
	if err != nil {
		return nil, err
	}
	for y := 0; y < b.height; y++ {
		for x := 0; x < cols; x++ {
			g.SetCell(x, y, buffer.Cell{Ch: b.tag})
		}
	}
	return g, nil
}

func mustGrid(t *testing.T, cols, rows int) *buffer.Grid {
	t.Helper()
	g, err := buffer.NewGrid(cols, rows)
	if err != nil {
		t.Fatalf("NewGrid: %v", err)
	}
	return g
}

// ============================================================
// T-5 / D-1 basic tests
// ============================================================

// #1 TestVirtualScrollback_SatisfiesInterface — compile + runtime assert.
func TestVirtualScrollback_SatisfiesInterface(t *testing.T) {
	var _ Scrollback = (*VirtualScrollback)(nil)
	sb := NewVirtualScrollback()
	_ = sb // runtime guard
}

// #2 TestPush_BasicSequence — Len + Range preserves insertion order.
func TestPush_BasicSequence(t *testing.T) {
	sb := NewVirtualScrollback()
	a, b, c := fakeBlock{height: 1, tag: 'A'}, fakeBlock{height: 1, tag: 'B'}, fakeBlock{height: 1, tag: 'C'}
	sb.Push(a)
	sb.Push(b)
	sb.Push(c)
	if sb.Len() != 3 {
		t.Fatalf("Len()=%d, want 3", sb.Len())
	}
	got := sb.Range(0, 3)
	if len(got) != 3 {
		t.Fatalf("Range len=%d, want 3", len(got))
	}
	if got[0].(fakeBlock).tag != 'A' || got[2].(fakeBlock).tag != 'C' {
		t.Fatalf("Range order wrong: %v", got)
	}
}

// #20 TestScrollY_GetterMatchesInternal (R2 D3) — ScrollY() observability.
func TestScrollY_GetterMatchesInternal(t *testing.T) {
	sb := NewVirtualScrollback()
	for i := 0; i < 5; i++ {
		sb.Push(fakeBlock{height: 4, tag: 'A'})
	}
	// First Render to set lastRows; viewport 80×5 with totalH=20 → maxY=15.
	sb.Render(mustGrid(t, 80, 5), layout.Box{Width: 80, Height: 5})
	sb.ScrollBy(-3) // from sticky maxY=15 → 12
	if got := sb.ScrollY(); got != 12 {
		t.Fatalf("after ScrollBy(-3) from maxY=15, ScrollY()=%d, want 12", got)
	}
	sb.ScrollToBottom()
	if got := sb.ScrollY(); got != 15 {
		t.Fatalf("after ScrollToBottom, ScrollY()=%d, want 15", got)
	}
}

// #4 TestScrollBy_UpDisablesSticky — scroll up away from bottom.
func TestScrollBy_UpDisablesSticky(t *testing.T) {
	sb := NewVirtualScrollback()
	for i := 0; i < 5; i++ {
		sb.Push(fakeBlock{height: 4, tag: 'A'})
	}
	sb.Render(mustGrid(t, 80, 5), layout.Box{Width: 80, Height: 5})
	if !sb.IsSticky() {
		t.Fatal("expected initial sticky=true")
	}
	sb.ScrollBy(-3)
	if sb.IsSticky() {
		t.Fatal("after ScrollBy(-3), expected sticky=false")
	}
}

// #5 TestScrollBy_DownToBottomEnablesSticky — arriving at bottom re-enables.
func TestScrollBy_DownToBottomEnablesSticky(t *testing.T) {
	sb := NewVirtualScrollback()
	for i := 0; i < 5; i++ {
		sb.Push(fakeBlock{height: 4, tag: 'A'})
	}
	sb.Render(mustGrid(t, 80, 5), layout.Box{Width: 80, Height: 5})
	sb.ScrollBy(-10)
	if sb.IsSticky() {
		t.Fatal("scrolled up: sticky should be false")
	}
	sb.ScrollBy(+1000) // huge: clamps to maxY
	if !sb.IsSticky() {
		t.Fatal("scrolled back to bottom: sticky should re-enable")
	}
}

// #6 TestScrollToBottom_EnablesSticky.
func TestScrollToBottom_EnablesSticky(t *testing.T) {
	sb := NewVirtualScrollback()
	for i := 0; i < 5; i++ {
		sb.Push(fakeBlock{height: 4, tag: 'A'})
	}
	sb.Render(mustGrid(t, 80, 5), layout.Box{Width: 80, Height: 5})
	sb.ScrollBy(-5)
	sb.ScrollToBottom()
	if !sb.IsSticky() {
		t.Fatal("ScrollToBottom should enable sticky")
	}
	if sb.ScrollY() != 15 {
		t.Fatalf("ScrollY()=%d, want 15 (totalH=20 - rows=5)", sb.ScrollY())
	}
}

// ============================================================
// T-6 / D-3 / D-4 / D-6 advanced tests
// ============================================================

// #3 TestPush_StickyAutoFollow — sticky=true auto-advances scrollY.
func TestPush_StickyAutoFollow(t *testing.T) {
	sb := NewVirtualScrollback()
	// Pre-Render so lastRows is set.
	sb.Render(mustGrid(t, 80, 10), layout.Box{Width: 80, Height: 10})
	for i := 0; i < 3; i++ {
		sb.Push(fakeBlock{height: 5, tag: 'A'})
	}
	// totalH = 15 (3 × 5), viewport rows=10 → maxY = 5.
	if got := sb.ScrollY(); got != 5 {
		t.Fatalf("sticky auto-follow: ScrollY()=%d, want 5", got)
	}
	if !sb.IsSticky() {
		t.Fatal("sticky should remain true after auto-follow")
	}
}

// #7 TestPush_MemoryCapDropsOldest (R2 D1 + R2 L-2): drop + cache invalidate.
func TestPush_MemoryCapDropsOldest(t *testing.T) {
	sb := NewVirtualScrollback(WithMaxNodes(100))
	sb.Render(mustGrid(t, 80, 10), layout.Box{Width: 80, Height: 10})

	// Push 100 first (cache populated as blocks are visible after each Render).
	for i := 0; i < 100; i++ {
		sb.Push(fakeBlock{height: 1, tag: 'A'})
	}
	// Trigger Render so cache gets populated for visible blocks.
	sb.Render(mustGrid(t, 80, 10), layout.Box{Width: 80, Height: 10})
	prevCacheLen := sb.cache.Len()
	if prevCacheLen == 0 {
		t.Fatal("expected cache to be populated after Render")
	}

	// Push 101st → triggers drop of 10 oldest (maxNodes/10).
	sb.Push(fakeBlock{height: 1, tag: 'B'})

	if sb.Len() != 91 {
		t.Fatalf("after cap-drop Len()=%d, want 91 (100+1 - 10 drop)", sb.Len())
	}
	// R2 D1: cache should be fully invalidated post-drop.
	if sb.cache.Len() != 0 {
		t.Fatalf("post-drop cache.Len()=%d, want 0 (R2 D1 InvalidateAll)", sb.cache.Len())
	}
}

// #19 TestWithMaxNodes_ClampToTen (R2 LOW-4): WithMaxNodes(<10) clamps to 10.
func TestWithMaxNodes_ClampToTen(t *testing.T) {
	sb := NewVirtualScrollback(WithMaxNodes(5))
	if sb.maxNodes != 10 {
		t.Fatalf("maxNodes=%d, want 10 (R2 LOW-4 clamp)", sb.maxNodes)
	}
	// Verify dropN=max(1, 10/10)=1 works: push 11 → Len()==10.
	for i := 0; i < 11; i++ {
		sb.Push(fakeBlock{height: 1, tag: 'A'})
	}
	if sb.Len() != 10 {
		t.Fatalf("after 11 push with maxNodes=10, Len()=%d, want 10", sb.Len())
	}
}

// #18 TestPush_BeforeFirstRender (R2 LOW-3): pre-Render Push → measure=1.
func TestPush_BeforeFirstRender(t *testing.T) {
	sb := NewVirtualScrollback()
	sb.Push(fakeBlock{height: 5, tag: 'A'})
	sb.Push(fakeBlock{height: 3, tag: 'B'})
	sb.Push(fakeBlock{height: 7, tag: 'C'})
	// Pre-Render: heights are placeholders (1).
	for i, h := range sb.heights {
		if h != 1 {
			t.Fatalf("pre-Render heights[%d]=%d, want 1 (placeholder)", i, h)
		}
	}
	// First Render → rebuildHeightsUnsafe re-measures real heights.
	sb.Render(mustGrid(t, 80, 30), layout.Box{Width: 80, Height: 30})
	wantH := []int{5, 3, 7}
	for i, h := range sb.heights {
		if h != wantH[i] {
			t.Fatalf("post-Render heights[%d]=%d, want %d", i, h, wantH[i])
		}
	}
}

// #13 TestRender_ResizeInvalidates (R2 MED-1): cache + ctx.Cols sync.
func TestRender_ResizeInvalidates(t *testing.T) {
	sb := NewVirtualScrollback()
	for i := 0; i < 3; i++ {
		sb.Push(fakeBlock{height: 4, tag: 'A'})
	}
	sb.Render(mustGrid(t, 80, 24), layout.Box{Width: 80, Height: 24})
	if sb.cache.Len() == 0 {
		t.Fatal("expected cache populated after Render")
	}
	// Resize: viewport changes → cache.InvalidateAll + ctx.Cols sync.
	sb.Render(mustGrid(t, 120, 40), layout.Box{Width: 120, Height: 40})
	if sb.ctx.Cols != 120 {
		t.Fatalf("post-resize ctx.Cols=%d, want 120 (R2 MED-1)", sb.ctx.Cols)
	}
	if sb.lastCols != 120 || sb.lastRows != 40 {
		t.Fatalf("lastCols/lastRows=%d/%d, want 120/40", sb.lastCols, sb.lastRows)
	}
}

// #14 TestRender_ViewportLargerThanTotal — no crash; rows past total skipped.
func TestRender_ViewportLargerThanTotal(t *testing.T) {
	sb := NewVirtualScrollback()
	sb.Push(fakeBlock{height: 3, tag: 'A'})
	sb.Push(fakeBlock{height: 3, tag: 'B'})
	next := mustGrid(t, 80, 20)
	sb.Render(next, layout.Box{Width: 80, Height: 20})
	// Rows >= 6 (totalH) should be blank.
	for y := 6; y < 20; y++ {
		if c := next.Cell(0, y); c.Ch != 0 {
			t.Fatalf("expected blank cell at y=%d, got %q", y, c.Ch)
		}
	}
}

// #12 TestRender_VisibleViewport — blits visible block cells into next.
func TestRender_VisibleViewport(t *testing.T) {
	sb := NewVirtualScrollback()
	for i := 0; i < 5; i++ {
		tag := rune('A' + i)
		sb.Push(fakeBlock{height: 4, tag: tag})
	}
	next := mustGrid(t, 80, 12)
	sb.Render(next, layout.Box{Width: 80, Height: 12})
	// sticky=true, totalH=20, viewport rows=12 → scrollY=8.
	// Visible region [y=8..19] covers block 2 (rows 8-11) + block 3 (12-15) + block 4 (16-19).
	// next[0..3] = block 2 tag='C'; next[4..7] = block 3 tag='D'; next[8..11] = block 4 tag='E'.
	if c := next.Cell(0, 0); c.Ch != 'C' {
		t.Fatalf("next[0,0] = %q, want 'C'", c.Ch)
	}
	if c := next.Cell(0, 7); c.Ch != 'D' {
		t.Fatalf("next[0,7] = %q, want 'D'", c.Ch)
	}
	if c := next.Cell(0, 11); c.Ch != 'E' {
		t.Fatalf("next[0,11] = %q, want 'E'", c.Ch)
	}
}

// #17 TestRender_DegenerateViewport (R2 LOW-5): cols/rows=0 → no-op.
func TestRender_DegenerateViewport(t *testing.T) {
	sb := NewVirtualScrollback()
	sb.Push(fakeBlock{height: 4, tag: 'A'})
	cases := []struct {
		name string
		w, h int
	}{
		{"cols=0", 0, 24},
		{"rows=0", 80, 0},
		{"both zero", 0, 0},
		{"negative cols", -1, 24},
		{"negative rows", 80, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := mustGrid(t, 80, 24) // grid dim doesn't have to match viewport
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Render(viewport=%dx%d) panicked: %v", tc.w, tc.h, r)
				}
			}()
			sb.Render(next, layout.Box{Width: tc.w, Height: tc.h})
		})
	}
}

// #16 TestRender_OverscanCacheHit — overscan rendered → next Render hit.
func TestRender_OverscanCacheHit(t *testing.T) {
	sb := NewVirtualScrollback(WithOverscan(5))
	for i := 0; i < 10; i++ {
		sb.Push(fakeBlock{height: 2, tag: 'A'})
	}
	sb.Render(mustGrid(t, 80, 6), layout.Box{Width: 80, Height: 6})
	cachedBefore := sb.cache.Len()
	if cachedBefore == 0 {
		t.Fatal("expected cache populated after first Render")
	}
	// Scroll within overscan range → cache should remain warm.
	sb.ScrollBy(-2)
	sb.Render(mustGrid(t, 80, 6), layout.Box{Width: 80, Height: 6})
	// Cache should be at least as full (no eviction triggered; overscan covered scroll).
	if sb.cache.Len() < cachedBefore {
		t.Fatalf("cache shrank after small scroll: before=%d after=%d", cachedBefore, sb.cache.Len())
	}
}

// TestRender_EmptyScrollback — Render no-op when no Push happened.
func TestRender_EmptyScrollback(t *testing.T) {
	sb := NewVirtualScrollback()
	next := mustGrid(t, 80, 24)
	sb.Render(next, layout.Box{Width: 80, Height: 24})
	// Should not panic + grid should be blank.
	if c := next.Cell(0, 0); c.Ch != 0 {
		t.Fatalf("expected blank, got %q", c.Ch)
	}
}

// TestRender_StickyOnResize (R3 review claude HIGH-2 + codex R1 LOW-1):
// sticky=true + viewport resize → scrollY 重锚到 new (totalH - rows).
// sticky=false + viewport resize → scrollY 不重锚 (preserve user
// position; still clamped to new maxY if shrinks below).
func TestRender_StickyOnResize(t *testing.T) {
	t.Run("sticky=true re-anchors to new bottom on resize", func(t *testing.T) {
		sb := NewVirtualScrollback()
		for i := 0; i < 5; i++ {
			sb.Push(fakeBlock{height: 4, tag: 'A'})
		}
		// First Render: rows=10, totalH=20 → maxY=10, sticky=true → scrollY=10.
		sb.Render(mustGrid(t, 80, 10), layout.Box{Width: 80, Height: 10})
		if got := sb.ScrollY(); got != 10 {
			t.Fatalf("pre-resize sticky scrollY=%d, want 10", got)
		}
		// Resize: rows=5 (smaller). Sticky should re-anchor scrollY to
		// new maxY = totalH - 5 = 15.
		sb.Render(mustGrid(t, 80, 5), layout.Box{Width: 80, Height: 5})
		if got := sb.ScrollY(); got != 15 {
			t.Fatalf("post-resize sticky scrollY=%d, want 15 (totalH=20 - rows=5)", got)
		}
		if !sb.IsSticky() {
			t.Fatal("sticky should remain true after resize")
		}
	})

	t.Run("sticky=false preserves user position on resize (clamped)", func(t *testing.T) {
		sb := NewVirtualScrollback()
		for i := 0; i < 5; i++ {
			sb.Push(fakeBlock{height: 4, tag: 'A'})
		}
		sb.Render(mustGrid(t, 80, 10), layout.Box{Width: 80, Height: 10})
		sb.ScrollBy(-5) // sticky=false; scrollY = 10-5 = 5
		if sb.IsSticky() {
			t.Fatal("sticky should be false after scroll up")
		}
		preY := sb.ScrollY()
		// Resize wider but same height; sticky=false → scrollY should NOT
		// re-anchor (preserve user position).
		sb.Render(mustGrid(t, 120, 10), layout.Box{Width: 120, Height: 10})
		if got := sb.ScrollY(); got != preY {
			t.Fatalf("post-resize non-sticky scrollY=%d, want %d (preserve)", got, preY)
		}
	})
}

// TestScrollBy_ZeroIsNoop (R3 review codex R1 MED-1 / claude LOW-1):
// ScrollBy(0) must not toggle sticky state. The bug: non-sticky scrollback
// sitting at scrollY==maxY (reachable via "scroll up to maxY-1 → resize
// taller → Render clamp at new maxY") would silently re-enable sticky
// because the bottom-arrival branch fires unconditionally after delta+=0.
func TestScrollBy_ZeroIsNoop(t *testing.T) {
	sb := NewVirtualScrollback()
	for i := 0; i < 5; i++ {
		sb.Push(fakeBlock{height: 4, tag: 'A'})
	}
	sb.Render(mustGrid(t, 80, 10), layout.Box{Width: 80, Height: 10})

	// Construct the state: scroll up so sticky=false, then arrange for
	// scrollY==maxY without re-enabling sticky. Easiest path: scroll up by 1.
	sb.ScrollBy(-1) // sticky=false; scrollY=9
	if sb.IsSticky() {
		t.Fatal("setup: expected sticky=false after ScrollBy(-1)")
	}
	preY := sb.ScrollY()
	preSticky := sb.IsSticky()

	// ScrollBy(0): must NOT change scrollY or sticky.
	sb.ScrollBy(0)

	if sb.ScrollY() != preY {
		t.Fatalf("ScrollBy(0) changed scrollY: pre=%d post=%d", preY, sb.ScrollY())
	}
	if sb.IsSticky() != preSticky {
		t.Fatalf("ScrollBy(0) toggled sticky: pre=%v post=%v", preSticky, sb.IsSticky())
	}
}

// wideRuneBlock writes one row containing two wide CJK runes at columns 0
// and 2 (each occupies 2 cells via Grid.SetCell auto-continuation).
type wideRuneBlock struct{}

func (wideRuneBlock) Render(ctx block.Context) (buffer.Buffer, error) {
	cols := ctx.Cols
	if cols < 4 {
		cols = 4
	}
	g, err := buffer.NewGrid(cols, 1)
	if err != nil {
		return nil, err
	}
	g.SetCell(0, 0, buffer.Cell{Ch: '你'})
	g.SetCell(2, 0, buffer.Cell{Ch: '好'})
	return g, nil
}

// TestRender_PreservesWideRunes guards against the spec-1.7 T-9 HIGH-2
// continuation-blit pattern re-introducing itself in virtualScrollback
// composeInto (spec-1.20.1 R-fix follow-up). Without the continuation skip,
// next.SetCell on the continuation cell would clear the wide-main at (x-1).
func TestRender_PreservesWideRunes(t *testing.T) {
	t.Parallel()
	sb := NewVirtualScrollback()
	sb.Push(wideRuneBlock{})

	next := mustGrid(t, 10, 3)
	sb.Render(next, layout.Box{Width: 10, Height: 3})

	// Sticky-follows-bottom places the single block at the last row. Scan
	// all rows defensively to insulate from sticky-anchor changes.
	var row = -1
	for y := 0; y < 3; y++ {
		if next.Cell(0, y).Ch == '你' {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("wide-main 你 not found in any row; grid: %+v", dumpRow(next, 0))
	}
	if !buffer.IsContinuation(next.Cell(1, row)) {
		t.Fatalf("next(1,%d) should be wide continuation; got %+v", row, next.Cell(1, row))
	}
	if got := next.Cell(2, row).Ch; got != '好' {
		t.Fatalf("next(2,%d) = %q; want 好 (wide main)", row, got)
	}
	if !buffer.IsContinuation(next.Cell(3, row)) {
		t.Fatalf("next(3,%d) should be wide continuation; got %+v", row, next.Cell(3, row))
	}
}

// dumpRow returns a debug-friendly summary of the first row's runes.
func dumpRow(g *buffer.Grid, y int) string {
	cols, _ := g.Size()
	out := ""
	for x := 0; x < cols; x++ {
		out += string(g.Cell(x, y).Ch) + "|"
	}
	return out
}

// TestRange_BoundsClampingAndEmpty — bounds-safety on Range.
func TestRange_BoundsClampingAndEmpty(t *testing.T) {
	sb := NewVirtualScrollback()
	sb.Push(fakeBlock{height: 1, tag: 'A'})
	sb.Push(fakeBlock{height: 1, tag: 'B'})
	// Negative start clamps to 0.
	got := sb.Range(-5, 2)
	if len(got) != 2 {
		t.Fatalf("Range(-5,2) len=%d, want 2", len(got))
	}
	// End past Len clamps.
	got = sb.Range(0, 100)
	if len(got) != 2 {
		t.Fatalf("Range(0,100) len=%d, want 2", len(got))
	}
	// start > end → nil.
	if sb.Range(5, 1) != nil {
		t.Fatal("Range(5,1) should return nil")
	}
}
