// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestWorker_BasicExecute — submit a noop cmd; verify worker runs it
// and writes a workerResult with no panic.
func TestWorker_BasicExecute(t *testing.T) {
	t.Parallel()
	p := newWorkerPool(2)
	defer p.Stop()

	var ran atomic.Bool
	j := jobItem{
		Cmd:       func() { ran.Store(true) },
		CmdID:     1,
		Submitted: time.Now(),
		Priority:  PriorityNormal,
	}
	if err := p.SubmitWithCtx(context.Background(), j); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case res := <-p.Results():
		if res.panicErr != nil {
			t.Errorf("unexpected panic: %v", res.panicErr)
		}
		if !ran.Load() {
			t.Errorf("Cmd did not execute")
		}
		if res.cmdID != 1 {
			t.Errorf("cmdID = %d, want 1", res.cmdID)
		}
	case <-time.After(time.Second):
		t.Fatal("workerResult timed out")
	}
}

// TestWorker_PanicRecovered — Cmd panic; worker recovers + writes
// workerResult{panicErr != nil}; subsequent Cmds still run (spec-1.4
// CRIT-C: 修 R1 defer-in-loop trap, panic 后 worker 不死).
func TestWorker_PanicRecovered(t *testing.T) {
	t.Parallel()
	p := newWorkerPool(1) // single worker to verify it survives
	defer p.Stop()

	// First Cmd panics.
	_ = p.SubmitWithCtx(context.Background(), jobItem{
		Cmd:       func() { panic("boom") },
		CmdID:     10,
		Submitted: time.Now(),
		Priority:  PriorityHigh,
	})
	res1 := <-p.Results()
	if res1.panicErr == nil {
		t.Fatalf("first Cmd should report panic; got %+v", res1)
	}
	if res1.stack == nil {
		t.Errorf("workerResult should carry stack")
	}

	// Second Cmd must still run — proves worker not dead.
	var ran atomic.Bool
	_ = p.SubmitWithCtx(context.Background(), jobItem{
		Cmd:       func() { ran.Store(true) },
		CmdID:     11,
		Submitted: time.Now(),
		Priority:  PriorityNormal,
	})
	res2 := <-p.Results()
	if res2.panicErr != nil {
		t.Errorf("second Cmd unexpectedly panicked: %v", res2.panicErr)
	}
	if !ran.Load() {
		t.Errorf("second Cmd did not execute — worker died after first panic (CRIT-C regression)")
	}
}

// TestWorker_StopCloseOrder — Submit then Stop; verify Stop completes
// (no deadlock) and all submitted Cmds executed before Stop returns
// (spec-1.4 R2 H-4 in-flight drain guarantee). With non-blocking
// results send, individual workerResult delivery is best-effort —
// what the contract guarantees is that close(jobs) lets workers
// drain the channel buffer and finish in-flight cmds before wg.Wait
// completes.
func TestWorker_StopCloseOrder(t *testing.T) {
	t.Parallel()
	p := newWorkerPool(2)
	var executed atomic.Int64
	for i := uint64(0); i < 10; i++ {
		_ = p.SubmitWithCtx(context.Background(), jobItem{
			Cmd:       func() { executed.Add(1) },
			CmdID:     i,
			Submitted: time.Now(),
			Priority:  PriorityNormal,
		})
	}

	stopReturned := make(chan struct{})
	go func() {
		p.Stop()
		close(stopReturned)
	}()

	select {
	case <-stopReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop deadlocked")
	}

	if got := executed.Load(); got != 10 {
		t.Errorf("executed = %d; want 10 (Stop must drain in-flight before returning)", got)
	}
}

// TestWorker_SubmitWithCtxCancel — fill jobs channel, then submit with
// cancelled ctx; verify SubmitWithCtx returns ctx.Err() promptly without
// blocking main loop (spec-1.4 R2 H-1 backpressure with escape).
func TestWorker_SubmitWithCtxCancel(t *testing.T) {
	t.Parallel()
	p := newWorkerPool(1) // 1 worker, jobs cap = 8
	defer p.Stop()

	// Block the single worker on a long Cmd so jobs channel fills.
	block := make(chan struct{})
	_ = p.SubmitWithCtx(context.Background(), jobItem{
		Cmd:      func() { <-block },
		CmdID:    1,
		Priority: PriorityHigh,
	})

	// Fill the buffered jobs channel (cap = 1*8 = 8) with quick cmds.
	for i := uint64(2); i <= 9; i++ {
		_ = p.SubmitWithCtx(context.Background(), jobItem{Cmd: func() {}, CmdID: i})
	}

	// Now jobs is full. Submit with a cancelled ctx; must return ctx.Err.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.SubmitWithCtx(ctx, jobItem{Cmd: func() {}, CmdID: 99})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("SubmitWithCtx err = %v; want context.Canceled", err)
	}

	close(block) // let worker drain so Stop completes cleanly
}

// TestWorker_SubmitAfterStop — submitting after Stop returns
// errPoolStopped without panicking on closed channel.
func TestWorker_SubmitAfterStop(t *testing.T) {
	t.Parallel()
	p := newWorkerPool(2)
	p.Stop()
	err := p.SubmitWithCtx(context.Background(), jobItem{Cmd: func() {}, CmdID: 1})
	if !errors.Is(err, errPoolStopped) {
		t.Errorf("Submit after Stop: err = %v, want errPoolStopped", err)
	}
}

// TestWorker_DurationRecorded — workerResult.duration is non-zero for
// real Cmds.
func TestWorker_DurationRecorded(t *testing.T) {
	t.Parallel()
	p := newWorkerPool(1)
	defer p.Stop()
	_ = p.SubmitWithCtx(context.Background(), jobItem{
		Cmd:       func() { time.Sleep(2 * time.Millisecond) },
		CmdID:     1,
		Submitted: time.Now(),
	})
	res := <-p.Results()
	if res.duration < time.Millisecond {
		t.Errorf("duration = %v, want > 1ms", res.duration)
	}
}
