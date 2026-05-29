// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File cache.go — fixed-capacity LRU ring of pool-owned cell grids
// indexed by block index. spec-1.5 D-5 (R2 D1 + D4 + D5 errata).
//
// Design (R2 errata):
//   - R2 D4: putFromBuffer accepts a buffer.Buffer src (block.Render
//     return type), internally Acquire's an owned *buffer.Grid from
//     the pool, copies cells from src into the owned grid, and stores
//     the owned grid in the LRU. The src buffer is not retained;
//     callers may discard / GC it after putFromBuffer returns.
//   - R2 D1: InvalidateAll releases every cached grid back to the pool
//     and clears the LRU + index. Called by memory-cap drop in
//     VirtualScrollback.Push (replaces broken invalidateRange — see
//     spec-1.5 § 6 R-5 + R2 D1 errata).
//   - R2 D5: lruRing has no internal mutex; caller (VirtualScrollback)
//     holds an outer Lock during cache operations (no RLock/Lock
//     upgrade path; no recursive locking). Cache mutates LRU + index
//     on every Get hit (move-to-front) and on Put/Evict.
//   - Cache entries store (cols, rows) alongside the grid so Get's
//     dimension check rejects stale entries after a resize before the
//     scrollback's invalidateAll runs (defense-in-depth; primary
//     invalidation happens in VirtualScrollback.Render's resize handler).
//
// Concurrency: NOT safe for concurrent use; outer VirtualScrollback.mu
// serializes all calls.

package scrollback

import (
	"container/list"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/paint"
)

// gridPool is the minimal subset of *buffer.BufferPool that lruRing
// depends on. Defining it as a local interface (rather than referencing
// *buffer.BufferPool directly) keeps cache tests decoupled from the
// real pool's sync.Pool internals — tests inject a counting fake to
// assert Acquire/Release call counts per the spec-1.2 D3 ownership
// contract.
type gridPool interface {
	Acquire(cols, rows int) (*buffer.Grid, error)
	Release(g *buffer.Grid)
}

// cacheEntry is one LRU node payload. Cols/rows are stored so Get can
// reject stale entries after a viewport resize (defense-in-depth; the
// primary resize-invalidate happens in VirtualScrollback.Render).
type cacheEntry struct {
	blockIdx int
	cols     int
	rows     int
	grid     *buffer.Grid
}

// lruRing is a fixed-capacity LRU cache of pool-owned *buffer.Grid
// keyed by block index. spec-1.5 D-5.
type lruRing struct {
	pool     gridPool
	capacity int
	lru      *list.List            // front = most recent; back = oldest
	index    map[int]*list.Element // blockIdx → list element holding *cacheEntry
}

// newLRURing constructs an empty cache with the given fixed capacity
// and pool reference. capacity must be > 0; callers (NewVirtualScrollback)
// ensure this by clamping the WithCacheSize option default to 16.
func newLRURing(capacity int, p gridPool) *lruRing {
	if capacity < 1 {
		capacity = 1
	}
	return &lruRing{
		pool:     p,
		capacity: capacity,
		lru:      list.New(),
		index:    make(map[int]*list.Element, capacity),
	}
}

// Len reports the current entry count. Helpful for tests + diagnostics.
func (c *lruRing) Len() int {
	return c.lru.Len()
}

// Get returns the cached grid for blockIdx if present AND its stored
// dimensions match (cols, rows). Cache hit moves the entry to the front
// (most-recent end). Miss returns (nil, false); stale-dim entries are
// treated as miss but NOT evicted here (resize handler in
// VirtualScrollback.Render calls InvalidateAll before Render iterates,
// so this path should not be hit in normal operation).
func (c *lruRing) Get(blockIdx, cols, rows int) (*buffer.Grid, bool) {
	el, ok := c.index[blockIdx]
	if !ok {
		return nil, false
	}
	e := el.Value.(*cacheEntry)
	if e.cols != cols || e.rows != rows {
		return nil, false
	}
	c.lru.MoveToFront(el)
	return e.grid, true
}

// putFromBuffer acquires an owned *buffer.Grid from the pool, copies
// every cell from src into it, and stores the owned grid under
// blockIdx. If blockIdx already has an entry, the old grid is released
// to the pool first (replace path). If the cache is at capacity, the
// oldest entry is evicted (its grid released) before insertion.
//
// Returns the newly stored grid so the caller (VirtualScrollback.Render)
// can blit from it without an extra Get round-trip. On Acquire failure
// (degenerate dims rejected by BufferPool), returns nil — caller treats
// it as cache miss + skips the block.
//
// R2 D4 errata: replaces the original Put(*Grid) signature which was
// type-incompatible with block.Render's buffer.Buffer return.
func (c *lruRing) putFromBuffer(blockIdx, cols, rows int, src buffer.Buffer) *buffer.Grid {
	if existing, ok := c.index[blockIdx]; ok {
		old := existing.Value.(*cacheEntry).grid
		c.pool.Release(old)
		c.lru.Remove(existing)
		delete(c.index, blockIdx)
	}
	if c.lru.Len() >= c.capacity {
		c.evictOldest()
	}
	owned, err := c.pool.Acquire(cols, rows)
	if err != nil {
		return nil
	}
	paint.Blit(owned, src, cols, rows)
	entry := &cacheEntry{blockIdx: blockIdx, cols: cols, rows: rows, grid: owned}
	el := c.lru.PushFront(entry)
	c.index[blockIdx] = el
	return owned
}

// InvalidateAll releases every cached grid back to the pool and clears
// the LRU + index. Called by VirtualScrollback.Push memory-cap drop
// (R2 D1) and by Render's resize handler (D-6).
func (c *lruRing) InvalidateAll() {
	for el := c.lru.Front(); el != nil; el = el.Next() {
		c.pool.Release(el.Value.(*cacheEntry).grid)
	}
	c.lru.Init()
	c.index = make(map[int]*list.Element, c.capacity)
}

// evictOldest removes the back element (oldest), releases its grid to
// the pool, and deletes the index entry. Internal helper; caller must
// ensure lru is non-empty.
func (c *lruRing) evictOldest() {
	back := c.lru.Back()
	if back == nil {
		return
	}
	e := back.Value.(*cacheEntry)
	c.pool.Release(e.grid)
	c.lru.Remove(back)
	delete(c.index, e.blockIdx)
}
