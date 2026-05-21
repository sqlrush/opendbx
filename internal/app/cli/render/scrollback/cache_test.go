// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scrollback

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// countingPool wraps *buffer.BufferPool counting Acquire/Release calls
// for cache lifetime test assertions (R2 D4 + spec-1.2 D3 ownership).
type countingPool struct {
	inner         *buffer.BufferPool
	acquireCalls  int
	releaseCalls  int
	releasedGrids []*buffer.Grid
}

func newCountingPool() *countingPool {
	return &countingPool{inner: buffer.NewBufferPool()}
}

func (p *countingPool) Acquire(cols, rows int) (*buffer.Grid, error) {
	p.acquireCalls++
	return p.inner.Acquire(cols, rows)
}

func (p *countingPool) Release(g *buffer.Grid) {
	p.releaseCalls++
	p.releasedGrids = append(p.releasedGrids, g)
	p.inner.Release(g)
}

// makeSrcBuffer builds a (cols×rows) buffer.Buffer (via real *Grid) where
// each cell carries rune = '0' + (x+y)%10 for distinguishable copy test.
func makeSrcBuffer(t *testing.T, cols, rows int) buffer.Buffer {
	t.Helper()
	g, err := buffer.NewGrid(cols, rows)
	if err != nil {
		t.Fatalf("NewGrid: %v", err)
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			g.SetCell(x, y, buffer.Cell{Ch: rune('0' + (x+y)%10)})
		}
	}
	return g
}

// TestNewLRURing_Defaults verifies constructor.
func TestNewLRURing_Defaults(t *testing.T) {
	p := newCountingPool()
	c := newLRURing(8, p)
	if c.Len() != 0 {
		t.Fatalf("new cache Len()=%d, want 0", c.Len())
	}
	if c.capacity != 8 {
		t.Fatalf("capacity=%d, want 8", c.capacity)
	}
}

// TestCache_PutFromBuffer_StoresOwnedGrid (R2 D4) verifies putFromBuffer
// internally Acquire's a pool grid + copies cells + stores owned grid.
func TestCache_PutFromBuffer_StoresOwnedGrid(t *testing.T) {
	p := newCountingPool()
	c := newLRURing(4, p)
	src := makeSrcBuffer(t, 8, 3)

	stored := c.putFromBuffer(0, 8, 3, src)

	if stored == nil {
		t.Fatal("putFromBuffer returned nil")
	}
	if p.acquireCalls != 1 {
		t.Fatalf("acquireCalls=%d, want 1 (one for owned grid)", p.acquireCalls)
	}
	if c.Len() != 1 {
		t.Fatalf("Len()=%d, want 1", c.Len())
	}
	// Verify cell content copied correctly.
	for y := 0; y < 3; y++ {
		for x := 0; x < 8; x++ {
			got := stored.Cell(x, y)
			want := rune('0' + (x+y)%10)
			if got.Ch != want {
				t.Fatalf("stored.Cell(%d,%d)=%q, want %q", x, y, got.Ch, want)
			}
		}
	}
}

// TestCache_Get_HitReturnsStoredGrid (R2 D4 + R2 D5 cache shape).
func TestCache_Get_HitReturnsStoredGrid(t *testing.T) {
	p := newCountingPool()
	c := newLRURing(4, p)
	src := makeSrcBuffer(t, 8, 3)
	stored := c.putFromBuffer(7, 8, 3, src)

	got, ok := c.Get(7, 8, 3)
	if !ok {
		t.Fatal("Get expected hit, got miss")
	}
	if got != stored {
		t.Fatal("Get returned different grid pointer than putFromBuffer")
	}
}

// TestCache_Get_MissUnknownKey verifies miss path.
func TestCache_Get_MissUnknownKey(t *testing.T) {
	p := newCountingPool()
	c := newLRURing(4, p)
	if _, ok := c.Get(42, 80, 24); ok {
		t.Fatal("Get on empty cache returned hit")
	}
}

// TestCache_Get_MissDimensionMismatch verifies cache entry stored at
// (cols=80, rows=24) does not satisfy Get(cols=120, rows=24). This is
// the resize-invalidation safety net (R-4 mitigation): cached grid for
// old width must not be served to caller expecting new width.
func TestCache_Get_MissDimensionMismatch(t *testing.T) {
	p := newCountingPool()
	c := newLRURing(4, p)
	src := makeSrcBuffer(t, 80, 24)
	c.putFromBuffer(5, 80, 24, src)

	if _, ok := c.Get(5, 120, 24); ok {
		t.Fatal("Get with mismatched cols returned hit; expected miss")
	}
	if _, ok := c.Get(5, 80, 30); ok {
		t.Fatal("Get with mismatched rows returned hit; expected miss")
	}
}

// TestCache_LRUEviction_ReleasesEvictedGrid (R2 D4 + spec-1.2 D3):
// cacheSize=2; putFromBuffer 3 different blockIdx → oldest evicted +
// pool.Release called for evicted grid.
func TestCache_LRUEviction_ReleasesEvictedGrid(t *testing.T) {
	p := newCountingPool()
	c := newLRURing(2, p)
	src := makeSrcBuffer(t, 4, 2)

	g0 := c.putFromBuffer(0, 4, 2, src)
	g1 := c.putFromBuffer(1, 4, 2, src)
	_ = c.putFromBuffer(2, 4, 2, src) // evicts blockIdx 0

	if c.Len() != 2 {
		t.Fatalf("Len()=%d, want 2 (capped)", c.Len())
	}
	if _, ok := c.Get(0, 4, 2); ok {
		t.Fatal("evicted entry blockIdx=0 still present")
	}
	if _, ok := c.Get(1, 4, 2); !ok {
		t.Fatal("blockIdx=1 should still be cached")
	}
	if p.releaseCalls != 1 {
		t.Fatalf("releaseCalls=%d, want 1 (only g0 evicted)", p.releaseCalls)
	}
	if len(p.releasedGrids) != 1 || p.releasedGrids[0] != g0 {
		t.Fatalf("expected pool.Release called with g0 (oldest); got %v", p.releasedGrids)
	}
	_ = g1
}

// TestCache_Get_HitMovesToFront (LRU ordering): Get-hit moves entry to
// recent end so it survives subsequent eviction.
func TestCache_Get_HitMovesToFront(t *testing.T) {
	p := newCountingPool()
	c := newLRURing(2, p)
	src := makeSrcBuffer(t, 4, 2)
	c.putFromBuffer(0, 4, 2, src)
	c.putFromBuffer(1, 4, 2, src)
	// Access blockIdx=0 → moves to front (most recent).
	if _, ok := c.Get(0, 4, 2); !ok {
		t.Fatal("expected Get hit on blockIdx=0")
	}
	// Now insert blockIdx=2 → evicts blockIdx=1 (oldest), not 0.
	c.putFromBuffer(2, 4, 2, src)
	if _, ok := c.Get(0, 4, 2); !ok {
		t.Fatal("blockIdx=0 should survive (moved to front by Get)")
	}
	if _, ok := c.Get(1, 4, 2); ok {
		t.Fatal("blockIdx=1 should be evicted (oldest)")
	}
}

// TestCache_InvalidateAll_ReleasesEntries (R2 D1): InvalidateAll → all
// entries released to pool + cache cleared.
func TestCache_InvalidateAll_ReleasesEntries(t *testing.T) {
	p := newCountingPool()
	c := newLRURing(4, p)
	src := makeSrcBuffer(t, 4, 2)
	for i := 0; i < 4; i++ {
		c.putFromBuffer(i, 4, 2, src)
	}
	if c.Len() != 4 {
		t.Fatalf("pre-invalidate Len()=%d, want 4", c.Len())
	}

	c.InvalidateAll()

	if c.Len() != 0 {
		t.Fatalf("post-invalidate Len()=%d, want 0", c.Len())
	}
	if p.releaseCalls != 4 {
		t.Fatalf("releaseCalls=%d, want 4 (all entries released)", p.releaseCalls)
	}
	// Subsequent Get should miss.
	if _, ok := c.Get(0, 4, 2); ok {
		t.Fatal("Get after InvalidateAll returned hit")
	}
}

// TestCache_PutFromBuffer_ReplaceSameKey verifies putFromBuffer with an
// already-cached blockIdx releases the old grid and stores the new one
// (Render after content change scenario; though MVP rarely hits this).
func TestCache_PutFromBuffer_ReplaceSameKey(t *testing.T) {
	p := newCountingPool()
	c := newLRURing(4, p)
	src := makeSrcBuffer(t, 4, 2)
	old := c.putFromBuffer(0, 4, 2, src)
	c.putFromBuffer(0, 4, 2, src) // replace

	if c.Len() != 1 {
		t.Fatalf("Len()=%d, want 1 (replace, not duplicate)", c.Len())
	}
	if p.releaseCalls != 1 {
		t.Fatalf("releaseCalls=%d, want 1 (old grid released)", p.releaseCalls)
	}
	if p.releasedGrids[0] != old {
		t.Fatal("expected old grid to be released on replace")
	}
}
