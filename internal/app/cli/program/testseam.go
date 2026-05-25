// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package program

import (
	"context"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// RunRenderForTest is the spec-1.15 D-7/D-8 test seam used by the
// integration visualgolden harness. It directly invokes Program.renderFn
// against the supplied grid, optionally setting the quit-armed state so
// fixtures like ProgramQuitArmed can be captured deterministically.
//
// Test code only. Production callers MUST use Program.Run (which wires
// the scheduler main loop). Marked test-only via godoc — moving to
// _test.go would prevent reuse from sibling test packages, so the helper
// lives in production source but with an explicit name suffix.
func RunRenderForTest(p *Program, grid *buffer.Grid, quitArmed bool) {
	p.quitArmed = quitArmed
	p.renderFn(grid)
}

// CurrentModelForTest returns the Program's current Model. spec-1.17
// D-7 integration test seam: handleMsg replaces p.model with each
// immutable Update result, so an external reference to the initial
// Model does NOT observe state advances. Integration tests read the
// live model through this accessor AFTER the scheduler loop has
// drained the injected key events.
//
// Test code only. NOT safe to call concurrently with a running
// scheduler goroutine — callers MUST cancel Run and join before
// reading (single-goroutine ownership of p.model; spec-1.15 R3).
func CurrentModelForTest(p *Program) Model {
	return p.model
}

// WaitStartedForTest blocks until the Program's scheduler has completed
// driver.Init (the Started signal). spec-1.17 D-7 integration harness:
// callers MUST wait on this before touching a shared SimulationScreen,
// since driver.Init runs sim.Init on the scheduler goroutine and tcell's
// SimulationScreen is not safe for concurrent Init vs other methods.
//
// Two-stage wait: first the schedSet channel (closed after p.scheduler
// is assigned in Run, making the p.scheduler read race-free), then the
// scheduler's Started channel (closed after driver.Init returns). The
// channel-close happens-before guarantees the caller observes Init's
// completion. Returns false if ctx is cancelled before readiness.
//
// Test code only.
func WaitStartedForTest(ctx context.Context, p *Program) bool {
	select {
	case <-p.schedSet:
	case <-ctx.Done():
		return false
	}
	select {
	case <-p.scheduler.Started():
		return true
	case <-ctx.Done():
		return false
	}
}
