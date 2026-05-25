// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scheduler

import (
	"sync"
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
		Cmd:       func() Msg { ran.Store(true); return nil },
		CmdID:     1,
		Submitted: time.Now(),
		Priority:  PriorityNormal,
	}
	if !p.TrySubmit(j) {
		t.Fatal("TrySubmit returned false on empty pool")
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

// TestWorker_PanicRecovered — spec-1.4 CRIT-C: runCmd's independent
// defer scope means a panic in one Cmd does NOT kill the worker;
// subsequent Cmds still run.
func TestWorker_PanicRecovered(t *testing.T) {
	t.Parallel()
	p := newWorkerPool(1) // single worker to verify it survives
	defer p.Stop()

	if !p.TrySubmit(jobItem{
		Cmd:       func() Msg { panic("boom") },
		CmdID:     10,
		Submitted: time.Now(),
		Priority:  PriorityHigh,
	}) {
		t.Fatal("first TrySubmit failed")
	}
	res1 := <-p.Results()
	if res1.panicErr == nil {
		t.Fatalf("first Cmd should report panic; got %+v", res1)
	}
	if res1.stack == nil {
		t.Errorf("workerResult should carry stack")
	}

	// Second Cmd must still run — proves worker not dead.
	var ran atomic.Bool
	if !p.TrySubmit(jobItem{
		Cmd:       func() Msg { ran.Store(true); return nil },
		CmdID:     11,
		Submitted: time.Now(),
		Priority:  PriorityNormal,
	}) {
		t.Fatal("second TrySubmit failed")
	}
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
// (spec-1.4 R2 H-4 in-flight drain guarantee).
func TestWorker_StopCloseOrder(t *testing.T) {
	t.Parallel()
	p := newWorkerPool(2)
	var executed atomic.Int64
	for i := uint64(0); i < 10; i++ {
		if !p.TrySubmit(jobItem{
			Cmd:       func() Msg { executed.Add(1); return nil },
			CmdID:     i,
			Submitted: time.Now(),
			Priority:  PriorityNormal,
		}) {
			t.Fatalf("TrySubmit #%d failed (channel full unexpectedly)", i)
		}
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

// TestWorker_TrySubmitFullReturnsFalse — spec-1.4 R3 H-1 + user 决策
// Option B: when the jobs channel is full, TrySubmit must return
// false promptly (no blocking, no ctx escape needed).
func TestWorker_TrySubmitFullReturnsFalse(t *testing.T) {
	t.Parallel()
	p := newWorkerPool(1) // 1 worker, jobs cap = 8
	defer p.Stop()

	// Block the single worker on a long Cmd so it can't drain the
	// channel during this test window. Use sync.Once+defer to ensure
	// close(block) fires even if t.Fatal short-circuits the test —
	// otherwise the worker hangs in runCmd, Stop's wg.Wait deadlocks.
	block := make(chan struct{})
	var unblockOnce sync.Once
	unblock := func() { unblockOnce.Do(func() { close(block) }) }
	defer unblock()
	if !p.TrySubmit(jobItem{
		Cmd:      func() Msg { <-block; return nil },
		CmdID:    1,
		Priority: PriorityHigh,
	}) {
		t.Fatal("initial TrySubmit failed")
	}
	// Wait for the worker to actually pick up Cmd 1 so the jobs
	// channel buffer is empty before we start filling it. Without this,
	// the racing fillers may collide with Cmd 1 still in the buffer
	// and the channel saturates before reaching the intended cap.
	time.Sleep(5 * time.Millisecond)

	// Fill the buffered jobs channel (cap = 1*8 = 8) with quick cmds.
	for i := uint64(2); i <= 9; i++ {
		if !p.TrySubmit(jobItem{Cmd: func() Msg { return nil }, CmdID: i}) {
			t.Fatalf("filler TrySubmit %d failed early — cap < 8?", i)
		}
	}

	// Channel full. Next TrySubmit must return false fast.
	start := time.Now()
	if got := p.TrySubmit(jobItem{Cmd: func() Msg { return nil }, CmdID: 99}); got {
		t.Errorf("TrySubmit on full channel returned true; want false")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Errorf("TrySubmit on full channel took %v; should be <1ms (non-blocking)", elapsed)
	}

	unblock() // explicit early unblock (defer is the safety net)
}

// TestWorker_TrySubmitAfterStop — TrySubmit after Stop returns false
// without panicking on closed channel.
func TestWorker_TrySubmitAfterStop(t *testing.T) {
	t.Parallel()
	p := newWorkerPool(2)
	p.Stop()
	if got := p.TrySubmit(jobItem{Cmd: func() Msg { return nil }, CmdID: 1}); got {
		t.Errorf("TrySubmit after Stop = true; want false")
	}
}

// TestWorker_TrySubmitConcurrentStopNoPanic — TrySubmit and Stop may
// race during scheduler shutdown. TrySubmit must return false rather
// than panicking with send-on-closed-channel.
func TestWorker_TrySubmitConcurrentStopNoPanic(t *testing.T) {
	t.Parallel()
	p := newWorkerPool(1)

	start := make(chan struct{})
	panicCh := make(chan any, 16)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(seed uint64) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCh <- r
				}
			}()
			<-start
			for i := uint64(0); i < 1000; i++ {
				_ = p.TrySubmit(jobItem{Cmd: func() Msg { return nil }, CmdID: seed*1000 + i})
			}
		}(uint64(g))
	}

	close(start)
	time.Sleep(time.Millisecond)
	p.Stop()
	wg.Wait()
	close(panicCh)
	if r, ok := <-panicCh; ok {
		t.Fatalf("TrySubmit raced with Stop and panicked: %v", r)
	}
}

// TestWorker_DurationRecorded — workerResult.duration is non-zero for
// real Cmds.
func TestWorker_DurationRecorded(t *testing.T) {
	t.Parallel()
	p := newWorkerPool(1)
	defer p.Stop()
	if !p.TrySubmit(jobItem{
		Cmd:       func() Msg { time.Sleep(2 * time.Millisecond); return nil },
		CmdID:     1,
		Submitted: time.Now(),
	}) {
		t.Fatal("TrySubmit failed")
	}
	res := <-p.Results()
	if res.duration < time.Millisecond {
		t.Errorf("duration = %v, want > 1ms", res.duration)
	}
}

// TestWorker_StopDrainsResults — spec-1.4 R3 H-C: when Stop is called
// while many in-flight results are unread (channel full + main loop
// stopped consuming), Stop's internal drain goroutine must unblock
// workers so wg.Wait completes (no goroutine leak / deadlock).
func TestWorker_StopDrainsResults(t *testing.T) {
	t.Parallel()
	p := newWorkerPool(2)

	// Submit enough cmds to overfill results channel (cap = 2*8 = 16).
	// Don't read results — simulate main loop exit.
	for i := uint64(0); i < 64; i++ {
		_ = p.TrySubmit(jobItem{Cmd: func() Msg { return nil }, CmdID: i, Submitted: time.Now()})
	}

	// Give workers a moment to start dropping results to default branch.
	time.Sleep(20 * time.Millisecond)

	stopReturned := make(chan struct{})
	go func() {
		p.Stop()
		close(stopReturned)
	}()
	select {
	case <-stopReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop deadlocked with unread results")
	}
}
