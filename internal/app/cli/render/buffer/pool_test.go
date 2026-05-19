// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package buffer

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestBufferPool_Acquire_ValidDims(t *testing.T) {
	t.Parallel()
	pool := NewBufferPool()
	cases := []struct {
		name       string
		cols, rows int
	}{
		{"small_80x24", 80, 24},
		{"medium_120x40", 120, 40},
		{"large_400x150", 400, 150},
		{"over_large", 600, 250}, // > LargeCapacity → direct alloc
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			g, err := pool.Acquire(c.cols, c.rows)
			if err != nil {
				t.Fatalf("Acquire(%d,%d) err = %v", c.cols, c.rows, err)
			}
			if g == nil {
				t.Fatal("Acquire returned nil Grid")
			}
			cols, rows := g.Size()
			if cols != c.cols || rows != c.rows {
				t.Errorf("Acquired Grid Size = %d,%d, want %d,%d", cols, rows, c.cols, c.rows)
			}
			// Acquired Grid is already blank: every cell reads zero.
			if got := g.Cell(0, 0); got != (Cell{}) {
				t.Errorf("freshly-acquired Cell(0,0) = %+v, want zero", got)
			}
			if got := g.Cell(c.cols-1, c.rows-1); got != (Cell{}) {
				t.Errorf("freshly-acquired Cell(corner) = %+v, want zero", got)
			}
		})
	}
}

func TestBufferPool_Acquire_InvalidDims(t *testing.T) {
	t.Parallel()
	pool := NewBufferPool()
	cases := []struct {
		name       string
		cols, rows int
	}{
		{"zero_cols", 0, 24},
		{"zero_rows", 80, 0},
		{"negative", -1, 24},
		{"overflow", 70_000, 70_000},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			g, err := pool.Acquire(c.cols, c.rows)
			if g != nil {
				t.Errorf("Acquire(%d,%d) grid = %p, want nil", c.cols, c.rows, g)
			}
			if !errors.Is(err, ErrInvalidDimension) {
				t.Errorf("Acquire(%d,%d) err = %v, want ErrInvalidDimension", c.cols, c.rows, err)
			}
		})
	}
}

// TestBufferPool_ReuseRoundTrip — Acquire, write, Release, Acquire
// again at the same size. The second Acquire must return a blank
// Grid even if the underlying *Grid instance is the one we just
// Released (sync.Pool warm-hit path).
func TestBufferPool_ReuseRoundTrip(t *testing.T) {
	t.Parallel()
	pool := NewBufferPool()
	g1, err := pool.Acquire(40, 12)
	if err != nil {
		t.Fatal(err)
	}
	g1.SetCell(0, 0, Cell{Ch: 'A'})
	g1.SetCell(39, 11, Cell{Ch: 'Z'})
	pool.Release(g1)

	g2, err := pool.Acquire(40, 12)
	if err != nil {
		t.Fatal(err)
	}
	// g2 must read blank everywhere.
	for y := 0; y < 12; y++ {
		for x := 0; x < 40; x++ {
			if got := g2.Cell(x, y); got != (Cell{}) {
				t.Errorf("reacquired Cell(%d,%d) = %+v, want zero (Release-then-Acquire blank invariant)",
					x, y, got)
				return
			}
		}
	}
}

// TestBufferPool_Release_ByBackingCapacity (spec-1.2 R2 LOW-1): a
// Grid originally medium-bucket that has Resized down to small
// dimensions must still go to the medium pool on Release, because
// the backing slice capacity is what matters for future Acquires.
func TestBufferPool_Release_ByBackingCapacity(t *testing.T) {
	t.Parallel()
	pool := NewBufferPool()
	// Acquire medium-sized.
	medium, err := pool.Acquire(150, 50) // 7500 cells → medium bucket
	if err != nil {
		t.Fatal(err)
	}
	wantCap := cap(medium.cells)
	if wantCap <= SmallCapacity {
		t.Skipf("medium acquire ended up with small backing cap=%d (allocator-dependent); skipping bucket-by-cap assertion",
			wantCap)
	}
	// Shrink to small dims without changing cols; Resize keeps the backing
	// slice capacity, so Release must still choose the medium pool by cap.
	medium.Resize(150, 5)
	if cap(medium.cells) <= SmallCapacity {
		t.Fatalf("post-shrink backing cap=%d, want > SmallCapacity to test bucket-by-cap", cap(medium.cells))
	}
	pool.Release(medium)

	// Re-acquire from the small bucket: must NOT receive our medium-
	// backed Grid (otherwise small pool is polluted).
	small, err := pool.Acquire(10, 5)
	if err != nil {
		t.Fatal(err)
	}
	if cap(small.cells) > SmallCapacity {
		t.Errorf("small Acquire returned Grid with cap=%d > SmallCapacity=%d — small pool polluted",
			cap(small.cells), SmallCapacity)
	}
}

func TestBufferPool_Release_Nil_NoOp(t *testing.T) {
	t.Parallel()
	pool := NewBufferPool()
	pool.Release(nil) // must not panic
}

// TestBufferPool_OverLarge_NotPooled — over-large backing slices are
// dropped on Release (GC'd) per R-1 mitigation. We cannot directly
// observe sync.Pool internals, but we can verify Release(over_large)
// does not panic and that the next over-large Acquire succeeds.
func TestBufferPool_OverLarge_NotPooled(t *testing.T) {
	t.Parallel()
	pool := NewBufferPool()
	g, err := pool.Acquire(600, 250) // > LargeCapacity
	if err != nil {
		t.Fatal(err)
	}
	pool.Release(g)
	g2, err := pool.Acquire(600, 250)
	if err != nil {
		t.Fatalf("over-large reacquire failed: %v", err)
	}
	if got := g2.Cell(0, 0); got != (Cell{}) {
		t.Errorf("over-large Cell(0,0) = %+v, want zero", got)
	}
}

// TestBufferPool_ConcurrentAcquireRelease — sync.Pool is safe for
// concurrent Acquire/Release; each goroutine owns its own Grid for
// the lifetime of its closure. The race detector must report 0
// findings.
func TestBufferPool_ConcurrentAcquireRelease(t *testing.T) {
	t.Parallel()
	pool := NewBufferPool()
	const goroutines = 16
	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var errs atomic.Int64
	for gi := 0; gi < goroutines; gi++ {
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				cols := 40 + (seed+i)%80 // varies across small/medium boundary
				rows := 12 + (seed+i)%30
				g, err := pool.Acquire(cols, rows)
				if err != nil {
					errs.Add(1)
					return
				}
				g.SetCell(0, 0, Cell{Ch: 'X'})
				g.SetCell(cols-1, rows-1, Cell{Ch: 'Y'})
				if g.Cell(0, 0).Ch != 'X' || g.Cell(cols-1, rows-1).Ch != 'Y' {
					errs.Add(1)
				}
				pool.Release(g)
			}
		}(gi)
	}
	wg.Wait()
	if errs.Load() != 0 {
		t.Errorf("concurrent Acquire/Release saw %d errors", errs.Load())
	}
}
