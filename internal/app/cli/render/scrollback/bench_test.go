// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Bench raw artifact target: tests/perf/stage1.5-scrollback-bench.txt
// (R2-2 raw pattern, no auto gate; T-8 wrap-up freeze baseline).
//
// spec § 4.3 targets (R2 MED-7 + MED-8 realistic):
//   - Push_1k (fakeBlock): < 100ms (full block.Render per measureHeight)
//   - Render_visible_viewport: < 1ms (cache hit + blit)
//   - ScrollBy_overscan_hit: < 100µs (no block.Render)
//   - Resize_10k: < 2s worst case (10k × full re-measure)

package scrollback

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/layout"
)

// BenchmarkScrollback_Push_1k pushes 1000 fakeBlocks (constant height 4)
// into a pre-rendered scrollback. Captures the measureHeight (full
// block.Render) + heights/offsets append cost path.
func BenchmarkScrollback_Push_1k(b *testing.B) {
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		sb := NewVirtualScrollback()
		// Prime so lastCols=80, measureHeight goes through real Render.
		g, _ := buffer.NewGrid(80, 24)
		sb.Render(g, layout.Box{Width: 80, Height: 24})
		b.StartTimer()
		for i := 0; i < 1000; i++ {
			sb.Push(fakeBlock{height: 4, tag: 'A'})
		}
	}
}

// BenchmarkScrollback_Render_visible_viewport renders a populated
// scrollback at the warm-cache scrollY=middle position.
func BenchmarkScrollback_Render_visible_viewport(b *testing.B) {
	sb := NewVirtualScrollback()
	g, _ := buffer.NewGrid(80, 24)
	sb.Render(g, layout.Box{Width: 80, Height: 24})
	for i := 0; i < 100; i++ {
		sb.Push(fakeBlock{height: 4, tag: 'A'})
	}
	sb.ScrollBy(-50) // park in the middle
	next, _ := buffer.NewGrid(80, 24)
	// Warm cache.
	sb.Render(next, layout.Box{Width: 80, Height: 24})
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		sb.Render(next, layout.Box{Width: 80, Height: 24})
	}
}

// BenchmarkScrollback_ScrollBy_overscan_hit alternates small ScrollBys
// within the overscan window so consecutive Renders hit cache.
func BenchmarkScrollback_ScrollBy_overscan_hit(b *testing.B) {
	sb := NewVirtualScrollback(WithOverscan(20))
	g, _ := buffer.NewGrid(80, 24)
	sb.Render(g, layout.Box{Width: 80, Height: 24})
	for i := 0; i < 100; i++ {
		sb.Push(fakeBlock{height: 4, tag: 'A'})
	}
	sb.ScrollBy(-50)
	next, _ := buffer.NewGrid(80, 24)
	sb.Render(next, layout.Box{Width: 80, Height: 24})
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		delta := 1
		if n%2 == 0 {
			delta = -1
		}
		sb.ScrollBy(delta)
		sb.Render(next, layout.Box{Width: 80, Height: 24})
	}
}

// BenchmarkScrollback_Resize_10k (R2 MED-8): worst-case rebuildHeights
// across maxNodes=10000 blocks on viewport size change.
func BenchmarkScrollback_Resize_10k(b *testing.B) {
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		sb := NewVirtualScrollback()
		g, _ := buffer.NewGrid(80, 24)
		sb.Render(g, layout.Box{Width: 80, Height: 24})
		for i := 0; i < 10000; i++ {
			sb.Push(fakeBlock{height: 2, tag: 'A'})
		}
		next, _ := buffer.NewGrid(120, 40)
		b.StartTimer()
		sb.Render(next, layout.Box{Width: 120, Height: 40}) // resize → rebuildHeights ×10k
	}
}
