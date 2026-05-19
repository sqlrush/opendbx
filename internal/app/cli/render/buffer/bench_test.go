// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package buffer

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
)

// BenchmarkSetCell_ASCII_Small runs SetCell over a 40×24 (small
// bucket) grid filling with ASCII runes — exercises the
// width.RuneWidth ASCII fast path.
func BenchmarkSetCell_ASCII_Small(b *testing.B) {
	g, _ := NewGrid(40, 24)
	cell := Cell{Ch: 'a', St: style.Style{}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for y := 0; y < 24; y++ {
			for x := 0; x < 40; x++ {
				g.SetCell(x, y, cell)
			}
		}
	}
}

// BenchmarkSetCell_ASCII_1000x1000 runs SetCell over a 1000×1000
// grid — the spec-1.2 D-5 acceptance size for the ASCII fast path.
func BenchmarkSetCell_ASCII_1000x1000(b *testing.B) {
	g, _ := NewGrid(1000, 1000)
	cell := Cell{Ch: 'a'}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for y := 0; y < 1000; y++ {
			for x := 0; x < 1000; x++ {
				g.SetCell(x, y, cell)
			}
		}
	}
}

// BenchmarkSetCell_CJK_Small fills a 40×24 grid with CJK runes
// (each occupies 2 cells via continuation write) — exercises the
// width.RuneWidth runewidth path. CJK fill walks x by 2 to avoid
// overwriting the continuation cell mid-iteration.
func BenchmarkSetCell_CJK_Small(b *testing.B) {
	g, _ := NewGrid(40, 24)
	cell := Cell{Ch: '中'}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for y := 0; y < 24; y++ {
			for x := 0; x < 40; x += 2 {
				g.SetCell(x, y, cell)
			}
		}
	}
}

// BenchmarkSetCell_CJK_1000x1000 fills a 1000×1000 grid with CJK runes.
func BenchmarkSetCell_CJK_1000x1000(b *testing.B) {
	g, _ := NewGrid(1000, 1000)
	cell := Cell{Ch: '中'}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for y := 0; y < 1000; y++ {
			for x := 0; x < 1000; x += 2 {
				g.SetCell(x, y, cell)
			}
		}
	}
}

// BenchmarkReset_1000x1000 bumps generation on a 1000×1000 grid — should
// be O(1) regardless of grid size (target: ≥ 100× faster than the
// memset baseline below).
func BenchmarkReset_1000x1000(b *testing.B) {
	g, _ := NewGrid(1000, 1000)
	// Pre-fill so each Reset has live cells to invalidate.
	for y := 0; y < 1000; y++ {
		for x := 0; x < 1000; x++ {
			g.SetCell(x, y, Cell{Ch: 'a'})
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Reset()
	}
}

// BenchmarkReset_Memset_1000x1000_Baseline emulates the would-be O(N)
// memset strategy on a 1000×1000 grid for comparison with BenchmarkReset_1000x1000.
func BenchmarkReset_Memset_1000x1000_Baseline(b *testing.B) {
	g, _ := NewGrid(1000, 1000)
	for y := 0; y < 1000; y++ {
		for x := 0; x < 1000; x++ {
			g.SetCell(x, y, Cell{Ch: 'a'})
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range g.cells {
			g.cells[j] = Cell{}
		}
		for j := range g.cellGen {
			g.cellGen[j] = 0
		}
	}
}

// BenchmarkBufferPool_AcquireRelease — typical scheduler frame-loop
// pattern: Acquire a medium grid, write a few cells, Release.
func BenchmarkBufferPool_AcquireRelease(b *testing.B) {
	pool := NewBufferPool()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g, err := pool.Acquire(120, 40)
		if err != nil {
			b.Fatal(err)
		}
		g.SetCell(0, 0, Cell{Ch: 'X'})
		g.SetCell(119, 39, Cell{Ch: 'Y'})
		pool.Release(g)
	}
}

// BenchmarkResize_Grow / BenchmarkResize_Shrink characterise the
// per-row copy path triggered when cols changes.
func BenchmarkResize_Grow(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g, _ := NewGrid(80, 24)
		g.Resize(120, 40)
	}
}

func BenchmarkResize_Shrink(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g, _ := NewGrid(200, 60)
		g.Resize(80, 24)
	}
}
