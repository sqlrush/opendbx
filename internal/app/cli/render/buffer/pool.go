// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package buffer

import (
	"math"
	"sync"
)

// BufferPool is the production sync.Pool for Grid instances
// (spec-1.2 D-2). It uses 3 size-bucketed sub-pools to reduce GC
// pressure for the typical TUI viewport range (80×24 standard,
// 120×40 expanded, 200×60 fullscreen, etc.) while avoiding the
// "park a 100 000-cell backing slice into the small pool" footgun
// that single-pool designs hit.
//
// Bucketing (by backing slice capacity, NOT by current cols×rows):
//
//   - small:  cap ≤ 80×24  = 1920    cells
//   - medium: cap ≤ 200×60 = 12000   cells
//   - large:  cap ≤ 500×200 = 100000 cells
//   - over-large: not pooled (GC'd on Release; bounded by R-1)
//
// Ownership transfer (spec-1.2 R2 D3, R-11):
//
//   - Acquire(cols, rows) returns ownership to the caller. The caller
//     MUST retain the returned *Grid until ALL downstream consumers
//     are done with it (typically: optimizer flush + scrollback
//     handoff + display flush). The pattern
//     `g := pool.Acquire(...); defer pool.Release(g); return g` is
//     a USE-AFTER-RELEASE bug — sync.Pool may hand the same instance
//     to a concurrent Acquire and data races / cell corruption will
//     follow.
//   - Release(g) returns the Grid to its size-appropriate sub-pool.
//     The caller must guarantee g is no longer read or written by
//     anyone.
//
// Concurrency: BufferPool itself is safe for concurrent Acquire /
// Release from multiple goroutines (sync.Pool semantics). Individual
// *Grid instances are NOT concurrent-safe (spec-0.13 D-3 single-owner
// contract).
type BufferPool struct {
	small  sync.Pool
	medium sync.Pool
	large  sync.Pool
}

// Bucket capacity bounds (cells). Public so tests / callers can
// reason about the partitioning explicitly.
const (
	SmallCapacity  = 80 * 24   // 1920
	MediumCapacity = 200 * 60  // 12000
	LargeCapacity  = 500 * 200 // 100000
)

// NewBufferPool constructs a BufferPool with the 3 size-bucketed
// sub-pools initialised. The sub-pools' New functions return nil;
// Acquire allocates fresh Grids on cold misses to ensure dimensions
// match the request.
func NewBufferPool() *BufferPool {
	return &BufferPool{
		small:  sync.Pool{New: func() any { return nil }},
		medium: sync.Pool{New: func() any { return nil }},
		large:  sync.Pool{New: func() any { return nil }},
	}
}

// Acquire returns an already-blank *Grid sized to (cols, rows). The
// caller becomes the single owner; see the BufferPool godoc for the
// release lifetime contract.
//
// Implementation:
//  1. validate dimensions (cols > 0, rows > 0, cols×rows fits int32);
//  2. select a sub-pool whose capacity bound is ≥ the requested
//     cells (or fall back to direct allocation if larger than
//     LargeCapacity);
//  3. drain a parked Grid from the sub-pool; if non-nil, Resize to
//     (cols, rows) (per spec-0.13 destructive contract) and call
//     Reset to bump generation for a clean frame; if nil, allocate
//     a fresh Grid (sized exactly to the request — does not pre-
//     allocate to the bucket bound to avoid wasting memory on small
//     viewports);
//  4. return ownership to the caller.
//
// Returns nil and ErrInvalidDimension for invalid dimensions.
func (p *BufferPool) Acquire(cols, rows int) (*Grid, error) {
	if cols <= 0 || rows <= 0 {
		return nil, ErrInvalidDimension
	}
	if int64(cols)*int64(rows) > int64(math.MaxInt32) {
		return nil, ErrInvalidDimension
	}
	needed := cols * rows
	sub := p.subPoolFor(needed)

	var g *Grid
	if sub != nil {
		if v := sub.Get(); v != nil {
			g = v.(*Grid)
		}
	}
	if g == nil {
		// Cold miss (or over-large fallback): allocate fresh.
		fresh, err := NewGrid(cols, rows)
		if err != nil {
			// errcode-lint:exempt -- spec-1.2 D-2: NewGrid only fails with ErrInvalidDimension (errcode.Sentinel propagation)
			return nil, err
		}
		return fresh, nil
	}
	// Warm hit: resize then Reset for a fresh frame. Resize preserves
	// surviving cellGen stamps; the immediate Reset bumps generation
	// so all of them go stale at once — caller sees blank Grid.
	g.Resize(cols, rows)
	g.Reset()
	return g, nil
}

// Release returns g to the appropriate sub-pool, chosen by the
// **backing slice capacity** `cap(g.cells)` rather than by the
// current `g.cols × g.rows` (spec-1.2 R2 LOW-1). A Grid that was
// originally medium-bucket but later Resized down to small
// dimensions still carries its medium backing slice; parking it in
// the small pool would inflate small-pool memory usage and surprise
// future small Acquires with oversized backings.
//
// Release(nil) is a no-op.
func (p *BufferPool) Release(g *Grid) {
	if g == nil {
		return
	}
	c := cap(g.cells)
	switch {
	case c == 0:
		// Empty / never-initialised — drop.
		return
	case c <= SmallCapacity:
		p.small.Put(g)
	case c <= MediumCapacity:
		p.medium.Put(g)
	case c <= LargeCapacity:
		p.large.Put(g)
	default:
		// Over-large backing — let GC reclaim instead of bloating
		// sync.Pool's P-local cache (mitigates R-1).
		return
	}
}

// subPoolFor selects the smallest sub-pool whose bound is ≥ needed
// cells, or nil if needed exceeds LargeCapacity (signalling a
// direct-alloc fallback).
func (p *BufferPool) subPoolFor(needed int) *sync.Pool {
	switch {
	case needed <= SmallCapacity:
		return &p.small
	case needed <= MediumCapacity:
		return &p.medium
	case needed <= LargeCapacity:
		return &p.large
	default:
		return nil
	}
}
