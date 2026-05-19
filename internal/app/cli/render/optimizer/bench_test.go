// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package optimizer

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// BenchmarkDiff_NoChange_1000x1000 — DiffEngine scans 1M cells, all
// equal: target the cell-by-cell value-compare hot path.
func BenchmarkDiff_NoChange_1000x1000(b *testing.B) {
	prev, _ := buffer.NewGrid(1000, 1000)
	next, _ := buffer.NewGrid(1000, 1000)
	for y := 0; y < 1000; y++ {
		for x := 0; x < 1000; x++ {
			prev.SetCell(x, y, buffer.Cell{Ch: 'a'})
			next.SetCell(x, y, buffer.Cell{Ch: 'a'})
		}
	}
	engine := NewDiffEngine()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.Diff(prev, next)
	}
}

// BenchmarkDiff_FullChange_1000x1000 — every cell differs: target the
// slice grow / 1M PatchSetCell append chain.
func BenchmarkDiff_FullChange_1000x1000(b *testing.B) {
	prev, _ := buffer.NewGrid(1000, 1000)
	next, _ := buffer.NewGrid(1000, 1000)
	for y := 0; y < 1000; y++ {
		for x := 0; x < 1000; x++ {
			prev.SetCell(x, y, buffer.Cell{Ch: 'a'})
			next.SetCell(x, y, buffer.Cell{Ch: 'b'})
		}
	}
	engine := NewDiffEngine()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.Diff(prev, next)
	}
}

// BenchmarkDiff_SparseChange_10pct_1000x1000 — ~10% cells differ:
// representative of typical TUI frame churn (decorations + content
// scroll partial updates).
func BenchmarkDiff_SparseChange_10pct_1000x1000(b *testing.B) {
	prev, _ := buffer.NewGrid(1000, 1000)
	next, _ := buffer.NewGrid(1000, 1000)
	for y := 0; y < 1000; y++ {
		for x := 0; x < 1000; x++ {
			prev.SetCell(x, y, buffer.Cell{Ch: 'a'})
			next.SetCell(x, y, buffer.Cell{Ch: 'a'})
		}
	}
	// Mutate every 10th cell in next.
	for y := 0; y < 1000; y++ {
		for x := 0; x < 1000; x += 10 {
			next.SetCell(x, y, buffer.Cell{Ch: 'b'})
		}
	}
	engine := NewDiffEngine()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.Diff(prev, next)
	}
}

// BenchmarkDiff_Resize_TriggerFullRedraw — size mismatch forces a
// PatchResize + fullRedraw of next.
func BenchmarkDiff_Resize_TriggerFullRedraw(b *testing.B) {
	prev, _ := buffer.NewGrid(80, 24)
	next, _ := buffer.NewGrid(120, 40)
	for y := 0; y < 40; y++ {
		for x := 0; x < 120; x++ {
			next.SetCell(x, y, buffer.Cell{Ch: 'a'})
		}
	}
	engine := NewDiffEngine()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.Diff(prev, next)
	}
}

// BenchmarkDiff_FirstFrame_NilPrev — nil-prev fullRedraw fast path; the
// clean-surface precondition (G1) means we only emit non-zero cells.
func BenchmarkDiff_FirstFrame_NilPrev(b *testing.B) {
	next, _ := buffer.NewGrid(1000, 1000)
	for y := 0; y < 1000; y++ {
		for x := 0; x < 1000; x++ {
			next.SetCell(x, y, buffer.Cell{Ch: 'a'})
		}
	}
	engine := NewDiffEngine()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.Diff(nil, next)
	}
}
