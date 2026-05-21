// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File virtual.go — production VirtualScrollback impl. spec-1.5 D-1 +
// D-3 (overscan) + D-4 (sticky bottom) + D-6 (resize handler).
//
// Public surface (spec-1.5 R2 D3 errata):
//
//	NewVirtualScrollback(opts ...Option) *VirtualScrollback
//	WithMaxNodes(n int) Option         // clamped to >=10 (R2 LOW-4)
//	WithOverscan(n int) Option
//	WithCacheSize(n int) Option
//
//	(*VirtualScrollback) Push(n block.RenderNode)                  // satisfies Scrollback
//	(*VirtualScrollback) Range(start, end int) []block.RenderNode  // satisfies Scrollback
//	(*VirtualScrollback) Len() int                                 // satisfies Scrollback
//	(*VirtualScrollback) Render(next *buffer.Grid, viewport layout.Box)  // R2 D3: no scrollY param
//	(*VirtualScrollback) ScrollBy(delta int)
//	(*VirtualScrollback) ScrollToBottom()
//	(*VirtualScrollback) IsSticky() bool
//	(*VirtualScrollback) ScrollY() int                             // R2 D3 getter
//
// Concurrency contract (R2 D5 matrix):
//   - Lock:  Push, ScrollBy, ScrollToBottom, Render
//   - RLock: Range, Len, IsSticky, ScrollY
//   - Cache has no internal mutex; outer s.mu serializes all cache calls.
//   - Render is main-goroutine-only (spec-1.4 RenderFn CRIT-A inherit);
//     internal Lock is defense-in-depth.

package scrollback

import (
	"sync"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/layout"
)

const (
	defaultMaxNodes  = 10000
	defaultOverscan  = 10
	defaultCacheSize = 16
	minMaxNodes      = 10 // R2 LOW-4: clamp WithMaxNodes(<10) so dropN >= 1
)

// Option configures NewVirtualScrollback via the functional options pattern.
type Option func(*VirtualScrollback)

// WithMaxNodes sets the maximum retained block count (clamped to >=10).
// Once Push pushes past the cap, the oldest max(1, maxNodes/10) blocks
// are dropped and the cache is fully invalidated (R2 D1).
func WithMaxNodes(n int) Option { return func(s *VirtualScrollback) { s.maxNodes = n } }

// WithOverscan sets the rows of pre-rendered context above + below the
// visible viewport (default 10). Larger values smooth scrolling at the
// cost of more cached buffers.
func WithOverscan(n int) Option { return func(s *VirtualScrollback) { s.overscan = n } }

// WithCacheSize sets the LRU buffer cache capacity (default 16).
func WithCacheSize(n int) Option { return func(s *VirtualScrollback) { s.cacheSize = n } }

// VirtualScrollback is the production Scrollback impl with offset table +
// binary search + overscan + sticky bottom + LRU buffer cache.
type VirtualScrollback struct {
	mu        sync.RWMutex
	nodes     []block.RenderNode // append-only chronological
	heights   []int              // parallel: heights[i] = node i row count
	offsets   []int              // sentinel prefix-sum: len(offsets)=len(nodes)+1, offsets[0]=0 (R2 D2)
	cache     *lruRing
	sticky    bool
	scrollY   int
	maxNodes  int
	overscan  int
	cacheSize int
	lastCols  int
	lastRows  int
	ctx       block.Context // for measure + render; ctx.Cols synced on resize (R2 MED-1)
}

// Compile-time assert that *VirtualScrollback satisfies the spec-0.13 D-1
// Scrollback interface.
var _ Scrollback = (*VirtualScrollback)(nil)

// NewVirtualScrollback constructs a VirtualScrollback with defaults
// (maxNodes=10000, overscan=10, cacheSize=16) overridable via opts.
// WithMaxNodes(n<10) is clamped to 10 (R2 LOW-4). offsets is initialized
// with sentinel head [0] (R2 D2 invariant: len(offsets)=len(nodes)+1).
func NewVirtualScrollback(opts ...Option) *VirtualScrollback {
	s := &VirtualScrollback{
		sticky:    true,
		maxNodes:  defaultMaxNodes,
		overscan:  defaultOverscan,
		cacheSize: defaultCacheSize,
		offsets:   []int{0}, // sentinel head
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.maxNodes < minMaxNodes {
		s.maxNodes = minMaxNodes // R2 LOW-4 clamp
	}
	s.cache = newLRURing(s.cacheSize, buffer.NewBufferPool())
	return s
}

// Push appends a node, measures its height at the current viewport width,
// and (if sticky) auto-advances scrollY to the new bottom. If the node
// count exceeds maxNodes, the oldest max(1, maxNodes/10) blocks are
// dropped and the cache is fully invalidated (R2 D1: invalidateRange
// would leave stale entries because surviving blockIdx shifts after
// reslice).
func (s *VirtualScrollback) Push(n block.RenderNode) {
	s.mu.Lock()
	defer s.mu.Unlock()

	h := s.measureHeightUnsafe(n)
	s.nodes = append(s.nodes, n)
	s.heights = append(s.heights, h)
	// Sentinel prefix-sum: offsets[i] = offsets[i-1] + heights[i-1].
	s.offsets = append(s.offsets, s.offsets[len(s.offsets)-1]+h)

	if s.sticky {
		s.scrollY = totalHeight(s.offsets) - s.lastRows
		if s.scrollY < 0 {
			s.scrollY = 0
		}
	}

	if len(s.nodes) > s.maxNodes {
		dropN := s.maxNodes / 10
		if dropN < 1 {
			dropN = 1
		}
		droppedHeight := 0
		for i := 0; i < dropN; i++ {
			droppedHeight += s.heights[i]
		}
		s.nodes = s.nodes[dropN:]
		s.heights = s.heights[dropN:]
		// R2 D1: must InvalidateAll because surviving entries' blockIdx
		// shifts (cache key becomes stale; invalidateRange leaks).
		s.cache.InvalidateAll()
		s.scrollY -= droppedHeight
		if s.scrollY < 0 {
			s.scrollY = 0
		}
		s.offsets = rebuildOffsets(s.heights)
	}
}

// Range returns a snapshot copy of nodes[start:end] (clamped).
func (s *VirtualScrollback) Range(start, end int) []block.RenderNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if start < 0 {
		start = 0
	}
	if end > len(s.nodes) {
		end = len(s.nodes)
	}
	if start > end {
		return nil
	}
	out := make([]block.RenderNode, end-start)
	copy(out, s.nodes[start:end])
	return out
}

// Len returns total node count (post-drop).
func (s *VirtualScrollback) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.nodes)
}

// ScrollBy adjusts scrollY by delta (positive=down, negative=up). The
// sticky state toggles based on the resulting position: arriving at the
// bottom enables sticky; scrolling up away from the bottom disables it.
//
// delta == 0 is a no-op (R3 review codex R1 MED-1 / claude LOW-1: the
// "arrival at bottom" branch fires unconditionally, so without this
// guard ScrollBy(0) at scrollY==maxY would silently re-enable sticky
// after the user had explicitly disabled it).
func (s *VirtualScrollback) ScrollBy(delta int) {
	if delta == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scrollY += delta
	if s.scrollY < 0 {
		s.scrollY = 0
	}
	maxY := totalHeight(s.offsets) - s.lastRows
	if maxY < 0 {
		maxY = 0
	}
	if s.scrollY >= maxY {
		s.scrollY = maxY
		s.sticky = true // arrived at bottom
	} else if delta < 0 {
		s.sticky = false // user scrolled up — pause auto-follow
	}
}

// ScrollToBottom snaps scrollY to bottom + enables sticky.
func (s *VirtualScrollback) ScrollToBottom() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sticky = true
	s.scrollY = totalHeight(s.offsets) - s.lastRows
	if s.scrollY < 0 {
		s.scrollY = 0
	}
}

// IsSticky reports whether sticky bottom is currently active.
// Safe from any goroutine (RLock).
func (s *VirtualScrollback) IsSticky() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sticky
}

// ScrollY returns the current scroll offset (line index). R2 D3 getter.
// Safe from any goroutine (RLock).
func (s *VirtualScrollback) ScrollY() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scrollY
}

// Render writes the visible portion of scrollback into next. Called
// from scheduler.RenderFn (main-loop only per spec-1.4 CRIT-A).
//
// R2 D3 errata: signature no longer takes external scrollY; the
// VirtualScrollback owns s.scrollY (mutated by ScrollBy / ScrollToBottom
// / Push-sticky). Callers observe via ScrollY().
//
// viewport.Width/Height define the visible window. Degenerate viewport
// (W<=0 || H<=0) returns immediately (R2 LOW-5). On resize (viewport
// size change), the cache is invalidated, s.ctx.Cols is synced (R2 MED-1),
// and heights/offsets are rebuilt. Overscan range above/below visible is
// rendered into cache for fast subsequent scrolls (R-4 mitigation).
//
// R2 D5 errata: takes Lock (not RLock) because cache Get/putFromBuffer
// mutates LRU + map; resize mutates heights/offsets/ctx.
func (s *VirtualScrollback) Render(next *buffer.Grid, viewport layout.Box) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cols, rows := viewport.Width, viewport.Height
	if cols <= 0 || rows <= 0 {
		return // R2 LOW-5 degenerate viewport
	}
	if cols != s.lastCols || rows != s.lastRows {
		s.cache.InvalidateAll()
		s.ctx.Cols = cols // R2 MED-1: sync ctx with new width
		s.ctx.Rows = rows
		s.rebuildHeightsUnsafe(cols)
		s.offsets = rebuildOffsets(s.heights)
		s.lastCols, s.lastRows = cols, rows
		// Sticky-on-resize: post-rebuild totalHeight may have changed
		// (heights re-measured at new width); honor sticky=true by
		// snapping scrollY to the new bottom — otherwise resize breaks
		// the "stay at bottom" mental model (less/tail-f). spec § 1.1
		// item 4 sticky state machine.
		if s.sticky {
			s.scrollY = totalHeight(s.offsets) - rows
			if s.scrollY < 0 {
				s.scrollY = 0
			}
		}
	}

	total := totalHeight(s.offsets)
	if total == 0 {
		return // empty scrollback
	}

	// Clamp s.scrollY to valid range (R2 D3: internal authority).
	maxY := total - rows
	if maxY < 0 {
		maxY = 0
	}
	if s.scrollY > maxY {
		s.scrollY = maxY
	}
	if s.scrollY < 0 {
		s.scrollY = 0
	}

	// Visible y range + overscan (D-3).
	renderTop := s.scrollY - s.overscan
	renderBot := s.scrollY + rows + s.overscan
	if renderTop < 0 {
		renderTop = 0
	}
	if renderBot > total {
		renderBot = total
	}

	startIdx := findBlockAt(s.offsets, renderTop)
	endIdx := findBlockAt(s.offsets, renderBot-1) + 1
	if endIdx > len(s.nodes) {
		endIdx = len(s.nodes)
	}

	for i := startIdx; i < endIdx; i++ {
		bufGrid, ok := s.cache.Get(i, cols, s.heights[i])
		if !ok {
			blockBuf, err := s.nodes[i].Render(s.ctx)
			if err != nil {
				continue // R-10: silent skip; caller responsibility to filter unsupported pre-Push
			}
			bufGrid = s.cache.putFromBuffer(i, cols, s.heights[i], blockBuf)
			if bufGrid == nil {
				continue // pool Acquire failed (degenerate dims); skip block
			}
		}
		// Blit bufGrid into next, offset by block's start y minus scrollY.
		blockStartY := s.offsets[i]
		for by := 0; by < s.heights[i]; by++ {
			destY := blockStartY + by - s.scrollY
			if destY < 0 || destY >= rows {
				continue
			}
			for bx := 0; bx < cols; bx++ {
				next.SetCell(bx, destY, bufGrid.Cell(bx, by))
			}
		}
	}
}

// measureHeightUnsafe returns the row count of node n when rendered at
// the current lastCols width. Caller must hold s.mu.
//
// MVP path: calls full block.Render(ctx) and reads buf.Size() rows.
// spec-1.7 forward contract will add ctx.MeasureOnly so this can skip
// the actual grid write (R-2 + R2 MED-6).
//
// Pre-first-Render Push (lastCols==0): returns 1 placeholder; Render's
// resize handler calls rebuildHeightsUnsafe to compute real heights once
// the viewport is known (R2 LOW-3 / MED-5).
//
// TODO(spec-1.7): pass block.Context{Cols: s.lastCols, MeasureOnly: true}
// once spec-1.7 block-interface adds the MeasureOnly field. Hard
// contract: block.Render MUST remain idempotent + side-effect-free +
// cheap (typical text block < 100µs) under spec-1.7 — if it cannot,
// spec-1.5's Push/measureHeight/rebuildHeights algorithm needs to be
// rewritten (not merely slower). See spec-1.5 § 6 R-2 + R3 review
// codex R2 MED-1 / claude MED-1.
func (s *VirtualScrollback) measureHeightUnsafe(n block.RenderNode) int {
	if s.lastCols == 0 {
		return 1 // pre-Render fallback
	}
	buf, err := n.Render(s.ctx)
	if err != nil {
		return 1 // R-10: silent skip path; caller responsibility
	}
	_, h := buf.Size()
	return h
}

// rebuildHeightsUnsafe re-measures every node's height at the new cols
// width. Caller must hold s.mu. O(N × block.Render cost) — invoked only
// on viewport resize, not per Push (R-6).
//
// TODO(spec-1.7): use block.Context{Cols: cols, MeasureOnly: true}
// to skip the actual grid write. Until spec-1.7 lands, this calls full
// block.Render for every node on every resize — worst case 10k nodes ×
// ~100µs ≈ 1s under the s.mu Lock. Also shares the idempotent / side-
// effect-free / cheap contract assumed by measureHeightUnsafe (see its
// TODO). spec-1.5 § 6 R-2 + R-6 amplify; R3 review claude HIGH-1.
func (s *VirtualScrollback) rebuildHeightsUnsafe(cols int) {
	if cols == 0 {
		return
	}
	for i, n := range s.nodes {
		buf, err := n.Render(s.ctx)
		if err != nil {
			s.heights[i] = 1
			continue
		}
		_, h := buf.Size()
		s.heights[i] = h
	}
}
