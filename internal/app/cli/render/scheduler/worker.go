// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scheduler

import (
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// workerResult is what each worker writes to the results channel after
// running one Cmd. It carries the jobItem context for ErrorMsg
// correlation (spec-1.4 R2 H-6) plus the recovered panic (if any), the
// wall-clock duration of the Cmd body, and (R3.2) the success Msg
// returned by the Cmd when it ran to completion.
type workerResult struct {
	cmdID      uint64
	submitted  time.Time
	priority   Priority
	duration   time.Duration
	panicErr   any    // non-nil → Cmd panicked; recover()'d value
	stack      []byte // non-nil iff panicErr != nil
	successMsg Msg    // R3.2: non-nil → Cmd returned a success Msg; main loop dispatches via msgHook
}

// workerPool is a fixed-size goroutine pool draining a buffered jobs
// channel. Each Cmd runs in its own runCmd defer scope (spec-1.4 R2
// CRIT-C: per-iteration recovery, not for-range defer-in-loop trap).
//
// Lifecycle: NewWorkerPool spawns N workers immediately. Stop must be
// called exactly once and follows the strict order documented on
// (*workerPool).Stop (spec-1.4 R2 H-4).
type workerPool struct {
	n       int
	jobs    chan jobItem
	results chan workerResult
	wg      sync.WaitGroup

	stopOnce sync.Once
	stopMu   sync.Mutex    // serializes TrySubmit sends with Stop closing jobs
	stopped  chan struct{} // closed by Stop; TrySubmit checks it
}

// newWorkerPool constructs and starts a worker pool with n workers.
// jobs channel cap = n*8 = 32 for typical n=4 (spec-1.4 R2 H-1).
func newWorkerPool(n int) *workerPool {
	if n <= 0 {
		n = 1
	}
	p := &workerPool{
		n:       n,
		jobs:    make(chan jobItem, n*8),
		results: make(chan workerResult, n*8),
		stopped: make(chan struct{}),
	}
	p.wg.Add(n)
	for i := 0; i < n; i++ {
		go p.workerLoop()
	}
	return p
}

// workerLoop drains the jobs channel until it is closed, invoking
// runCmd for each item. runCmd's own defer/recover scope isolates one
// Cmd's panic from the next (CRIT-C).
//
// Result-send is non-blocking: if the consumer (main loop) hasn't
// drained Results in time we'd otherwise risk a shutdown deadlock
// (workers blocked on results-send while Stop waits for them).
// Dropped results lose their ErrorMsg correlation context — the same
// drop policy as the msgCh emit (spec-1.4 R2 H-3); we log when the
// dropped result carried a panic so caller doesn't silently miss
// background failures.
func (p *workerPool) workerLoop() {
	defer p.wg.Done()
	for j := range p.jobs {
		res := runCmd(j)
		select {
		case p.results <- res:
		default:
			if res.panicErr != nil {
				slog.Warn("scheduler: worker result dropped (results channel full); ErrorMsg suppressed",
					"cmd_id", res.cmdID,
					"panic", res.panicErr)
			}
		}
	}
}

// runCmd executes one Cmd inside its own defer scope. Returns a
// workerResult whose panicErr field is non-nil iff cmd panicked.
//
// spec-1.4 R2 CRIT-C: this MUST be a standalone function (not an
// inlined closure inside workerLoop's for-range) so each Cmd gets a
// fresh defer chain. The original R1 design used
// `for cmd := range jobs { defer recover; cmd(); ... }` which
// accumulated defers until workerLoop returned and only caught the
// LAST panic — kill the worker on the first panic + subsequent Cmds
// never ran.
func runCmd(j jobItem) (res workerResult) {
	res.cmdID = j.CmdID
	res.submitted = j.Submitted
	res.priority = j.Priority

	defer func() {
		if r := recover(); r != nil {
			res.panicErr = r
			res.stack = debug.Stack()
		}
		res.duration = time.Since(j.Submitted)
	}()

	if j.Cmd != nil {
		// R3.2: Cmd returns Msg; non-nil Msg is forwarded as success result.
		res.successMsg = j.Cmd()
	}
	return res
}

// TrySubmit attempts a non-blocking enqueue of j on the jobs channel.
// Returns true on success, false if the channel is full or the pool
// has been stopped.
//
// spec-1.4 R2 H-1 + R3 user 决策 Option B: scheduler is the UI frame
// loop; it MUST NOT block on a full worker channel — that would stall
// frame rendering and break CC/Bubbletea-style responsiveness. The
// frame loop is expected to call queue.popIf with TrySubmit so success
// removes exactly the submitted head and failure leaves it queued.
// Backpressure is absorbed by the unbounded queue lane rather than
// the bounded channel.
func (p *workerPool) TrySubmit(j jobItem) bool {
	p.stopMu.Lock()
	defer p.stopMu.Unlock()

	select {
	case <-p.stopped:
		return false
	default:
	}
	select {
	case p.jobs <- j:
		return true
	default:
		return false
	}
}

// Results returns the read-only results channel. The main loop reads
// from this channel and emits ErrorMsg for any workerResult with a
// non-nil panicErr.
func (p *workerPool) Results() <-chan workerResult { return p.results }

// Stop shuts the pool down in the order required by spec-1.4 R2 H-4:
//
//  1. close(stopped) — concurrent TrySubmit observes shutdown
//  2. close(jobs)    — workers' for-range loops exit naturally
//  3. drain results  — background goroutine drops any pending sends
//     so wg.Wait can complete (workers MAY have
//     in-flight `p.results <- runCmd(j)` writes when
//     the main loop has already exited its select
//     and is no longer reading p.Results)
//  4. wg.Wait()      — all workers exited
//  5. close(results) — final close after all sends are done
func (p *workerPool) Stop() {
	p.stopOnce.Do(func() {
		p.stopMu.Lock()
		close(p.stopped)
		close(p.jobs)
		p.stopMu.Unlock()

		// Concurrent drain: prevents wg.Wait deadlocking when the
		// main loop has stopped consuming p.Results before Stop is
		// called.
		drainDone := make(chan struct{})
		go func() {
			for range p.results {
				// drop
			}
			close(drainDone)
		}()

		p.wg.Wait()
		close(p.results)
		<-drainDone
	})
}
