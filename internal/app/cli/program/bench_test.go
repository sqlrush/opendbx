// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package program

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// spec-1.15 § 4.3 perf targets:
//   - BenchmarkProgram_60fps_idle              < 500µs/frame (scheduler budget)
//   - BenchmarkProgram_keypress_to_view        < 200µs (KeyMsg → Update → next View)
//   - BenchmarkProgram_resize                  < 1ms (ResizeMsg → layout recompute → re-render)
//
// These benches exercise renderFn + handleMsg directly without running
// the scheduler loop, isolating the program-layer cost. Real-world
// throughput is bounded by the scheduler 60fps ticker.

func BenchmarkProgram_60fps_idle(b *testing.B) {
	b.ReportAllocs()
	drv := newFakeDriver(80, 24)
	m := &testModel{}
	p := New(drv, m)
	grid, _ := buffer.NewGrid(80, 24)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.renderFn(grid)
	}
}

func BenchmarkProgram_keypress_to_view(b *testing.B) {
	b.ReportAllocs()
	drv := newFakeDriver(80, 24)
	m := &testModel{}
	p := New(drv, m)
	grid, _ := buffer.NewGrid(80, 24)
	// Pre-wire a scheduler so EmitMsg/EmitError don't NPE in panic paths
	// (handleMsg uses p.scheduler.EmitError on panic; we never trigger
	// panic here but the call site references p.scheduler). For bench
	// purposes a minimal scheduler is fine — we only invoke handleMsg.
	// Use a sched-less path by not exercising panic.
	key := KeyMsg{Code: terminal.KeyRune, Rune: 'a'}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.handleMsgNoSchedule(key)
		p.renderFn(grid)
	}
}

func BenchmarkProgram_resize(b *testing.B) {
	b.ReportAllocs()
	drv := newFakeDriver(80, 24)
	m := &testModel{}
	p := New(drv, m)
	grid80, _ := buffer.NewGrid(80, 24)
	grid120, _ := buffer.NewGrid(120, 40)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			p.renderFn(grid120)
		} else {
			p.renderFn(grid80)
		}
	}
}

// handleMsgNoSchedule is a bench-only variant that exercises the
// Update + model-replace path without dispatching the returned Cmd via
// the scheduler. Production handleMsg routes Cmd through
// p.scheduler.Schedule which requires a live scheduler.
func (p *Program) handleMsgNoSchedule(msg interface{}) {
	if p.preDispatchSystemNoSchedule(msg) {
		return
	}
	newModel, _ := p.model.Update(msg)
	if newModel != nil {
		p.model = newModel
	}
}

func (p *Program) preDispatchSystemNoSchedule(msg interface{}) (handled bool) {
	switch msg.(type) {
	case QuitMsg, quitDisarmMsg:
		return true
	}
	return false
}
