// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// BenchmarkFrame_60fps_NoChange_baseline measures one tick of the
// frame loop when neither the model state nor the buffer dimensions
// have changed — the dominant cost is DiffEngine NoChange + Driver.Show.
//
// We invoke runFrame directly (rather than Run + ticker) so the bench
// reports per-frame cost without timer/scheduling jitter.
func BenchmarkFrame_60fps_NoChange_baseline(b *testing.B) {
	drv := &mockDriver{cols: 80, rows: 24}
	render := func(g *buffer.Grid) {
		// Static content; same across frames.
		for y := 0; y < 24; y++ {
			for x := 0; x < 80; x++ {
				g.SetCell(x, y, buffer.Cell{Ch: 'a'})
			}
		}
	}
	s := NewFrameScheduler(drv, 60, render)
	if err := drv.Init(); err != nil {
		b.Fatal(err)
	}
	defer drv.Fini()
	defer s.workers.Stop()
	defer close(s.msgCh)

	// Warm-up to populate lastFrame (so subsequent frames are NoChange).
	s.runFrame(context.Background())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.runFrame(context.Background())
	}
}

// BenchmarkFrame_60fps_FullChange_budget — every cell differs every
// frame; expected to exceed the 16.67 ms frame budget given spec-1.3
// FullChange diff is ~13.6 ms. Recorded as raw baseline (R2-2 pattern).
func BenchmarkFrame_60fps_FullChange_budget(b *testing.B) {
	drv := &mockDriver{cols: 80, rows: 24}
	tick := 0
	render := func(g *buffer.Grid) {
		tick++
		ch := rune('a' + tick%26)
		for y := 0; y < 24; y++ {
			for x := 0; x < 80; x++ {
				g.SetCell(x, y, buffer.Cell{Ch: ch})
			}
		}
	}
	s := NewFrameScheduler(drv, 60, render)
	if err := drv.Init(); err != nil {
		b.Fatal(err)
	}
	defer drv.Fini()
	defer s.workers.Stop()
	defer close(s.msgCh)

	s.runFrame(context.Background()) // warm-up
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.runFrame(context.Background())
	}
}

// BenchmarkWorkerPool_Throughput — submit noop Cmds and measure per-Cmd
// throughput through the worker pool (channel send + recv + runCmd
// defer). Approximates the cost of dispatching async background work.
func BenchmarkWorkerPool_Throughput(b *testing.B) {
	p := newWorkerPool(4)
	defer p.Stop()

	// Drain results in background so workers don't block on full
	// results channel during sustained throughput.
	stopDrain := make(chan struct{})
	go func() {
		for {
			select {
			case <-p.Results():
			case <-stopDrain:
				return
			}
		}
	}()
	defer close(stopDrain)

	ctx := context.Background()
	now := time.Now()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.SubmitWithCtx(ctx, jobItem{
			Cmd:       func() {},
			CmdID:     uint64(i),
			Submitted: now,
			Priority:  PriorityNormal,
		})
	}
}
