// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/optimizer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// RenderFn is the caller-provided closure invoked by the FrameScheduler
// main loop to populate the freshly-acquired `next` Grid before the
// optimizer diff (spec-1.4 R2 CRIT-A 用户决策 X).
//
// MAIN-LOOP-ONLY CONTRACT: RenderFn is invoked exclusively from the
// FrameScheduler.Run goroutine. Worker goroutines MUST NOT invoke
// RenderFn or directly mutate any *buffer.Grid handed out by the pool
// — spec-1.2 declares Grid "NOT safe for concurrent use" (single-owner;
// grid.go:21). Cmds dispatched via Schedule/ScheduleAt are free to
// compute state asynchronously but must hand results back via the
// caller's own thread-safe state container (e.g. mutex-guarded model);
// RenderFn then reads that snapshot when it runs on the main loop.
type RenderFn func(next *buffer.Grid)

// Msg is the marker interface for messages flowing on FrameScheduler.msgCh.
type Msg interface{ msg() }

// CmdID is the opaque correlation token returned by Schedule /
// ScheduleAt. Caller uses it to correlate a later ErrorMsg to the
// original Cmd that panicked.
type CmdID uint64

// ErrorMsg carries a recovered Cmd panic with full context for caller
// debugging (spec-1.4 R2 H-6). CmdID matches the value returned by
// Schedule/ScheduleAt; Frame is the scheduler frame counter when the
// main loop observes and emits the recovered worker result; Submitted/
// Priority are forwarded from the original jobItem.
type ErrorMsg struct {
	CmdID     CmdID
	Err       error
	Stack     []byte
	Submitted time.Time
	Priority  Priority
	Frame     int
}

func (ErrorMsg) msg() {}

// FrameScheduler is the production Scheduler impl driving the render
// frame loop at a configurable FPS (default 60). spec-1.4 D-1.
//
// Concurrency: Run owns the render goroutine. lastFrame, frame, and
// the per-frame ownership state machine live entirely on that goroutine
// (no atomics needed). Schedule/ScheduleAt go through the
// mutex-protected queue and are safe from any goroutine.
type FrameScheduler struct {
	driver        terminal.Driver
	pool          *buffer.BufferPool
	diff          *optimizer.DiffEngine
	queue         *queue
	workers       *workerPool
	render        RenderFn
	fps           int
	frameDeadline time.Duration
	tickCh        chan Tick
	msgCh         chan Msg
	lastFrame     *buffer.Grid
	frame         int
	nextCmdID     atomic.Uint64
}

// Compile-time assertion that *FrameScheduler satisfies Scheduler.
var _ Scheduler = (*FrameScheduler)(nil)

// NewFrameScheduler constructs a FrameScheduler.
//
// driver MUST be non-nil. render MUST be non-nil (spec-1.4 R2 CRIT-A
// — silently rendering blank frames hides a caller wiring bug; we
// panic at construction time with a clear message instead). fps <= 0
// is clamped to 60.
func NewFrameScheduler(driver terminal.Driver, fps int, render RenderFn) *FrameScheduler {
	if driver == nil {
		panic("scheduler.NewFrameScheduler: driver must be non-nil (caller lifecycle bug)")
	}
	if render == nil {
		panic("scheduler.NewFrameScheduler: render must be non-nil — spec-1.4 R2 CRIT-A; pass a RenderFn closure that populates the next *buffer.Grid from your model state")
	}
	if fps <= 0 {
		fps = 60
	}
	return &FrameScheduler{
		driver:        driver,
		pool:          buffer.NewBufferPool(),
		diff:          optimizer.NewDiffEngine(),
		queue:         newQueue(),
		workers:       newWorkerPool(4),
		render:        render,
		fps:           fps,
		frameDeadline: time.Second / time.Duration(fps),
		tickCh:        make(chan Tick, 1),
		msgCh:         make(chan Msg, 16),
	}
}

// Schedule enqueues cmd at PriorityNormal as fire-and-forget. The
// Scheduler interface signature (spec-0.13 D-1 FROZEN) returns nothing,
// so callers that need a CmdID for ErrorMsg correlation must use
// ScheduleAt instead (spec-1.4 R3 H-B + go-reviewer MED-1: godoc
// honesty — signature can't return CmdID without breaking spec-0.13).
func (s *FrameScheduler) Schedule(cmd Cmd) {
	s.ScheduleAt(cmd, PriorityNormal)
}

// ScheduleAt enqueues cmd at the given priority lane.
func (s *FrameScheduler) ScheduleAt(cmd Cmd, p Priority) CmdID {
	id := CmdID(s.nextCmdID.Add(1))
	s.queue.push(jobItem{
		Cmd:       cmd,
		CmdID:     uint64(id),
		Submitted: time.Now(),
		Priority:  p,
	})
	return id
}

// Tick returns the read-only tick channel. Each successful frame sends
// one Tick (non-blocking; tick dropped if consumer is slow).
func (s *FrameScheduler) Tick() <-chan Tick { return s.tickCh }

// Msgs returns the read-only message channel. The channel is closed
// when Run returns, so a for-range consumer exits cleanly (spec-1.4 R2
// MED-2 goroutine leak fix).
func (s *FrameScheduler) Msgs() <-chan Msg { return s.msgCh }

// emit sends m to msgCh with the spec-1.4 R2 H-3 "non-blocking drop-
// oldest" policy: if the channel is full we discard one queued message
// and retry; if even that fails (extreme contention) we log and drop
// the new message rather than block the main loop.
func (s *FrameScheduler) emit(m Msg) {
	select {
	case s.msgCh <- m:
		return
	default:
	}
	// Drop one oldest message to make room
	select {
	case <-s.msgCh:
	default:
	}
	select {
	case s.msgCh <- m:
	default:
		slog.Warn("scheduler.msgCh saturated; dropping message", "msg_type", fmt.Sprintf("%T", m))
	}
}

// Run drives the frame loop until ctx is cancelled. Single goroutine;
// callers MUST launch in their own goroutine. Returns driver.Init's
// error or ctx.Err() on clean shutdown. msgCh is closed before return.
func (s *FrameScheduler) Run(ctx context.Context) error {
	// errcode-lint:exempt -- spec-1.4 D-1: terminal.Driver.Init returns terminal-layer errors verbatim (G1 clean-surface precondition); scheduler is a pass-through, not the error origin.
	if err := s.driver.Init(); err != nil {
		return err
	}
	defer s.driver.Fini()
	defer s.workers.Stop()
	defer close(s.msgCh)
	// spec-1.4 R3 M-A: release the final adopted lastFrame back to the
	// pool on Run exit, otherwise the pool slowly leaks one Grid per
	// scheduler lifecycle.
	defer func() {
		if s.lastFrame != nil {
			s.pool.Release(s.lastFrame)
			s.lastFrame = nil
		}
	}()

	ticker := time.NewTicker(s.frameDeadline)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// errcode-lint:exempt -- spec-1.4 D-1: ctx.Err returns context.Canceled / DeadlineExceeded verbatim by design (stdlib sentinel pass-through, matches spec-0.12 D-3 tui.Run pattern).
			return ctx.Err()

		case <-ticker.C:
			s.runFrame(ctx)
		case res, ok := <-s.workers.Results():
			if !ok {
				// results closed during shutdown; continue, ctx will fire.
				continue
			}
			if res.panicErr != nil {
				// spec-1.4 R3 H-A: wrap with %w so callers can use
				// errors.Is(em.Err, ErrPanicRecovered) to programmatically
				// detect scheduler panic-recovered errors (rule 7 errcode
				// chain). %v alone produced a string-only error and made
				// the registered sentinel effectively dead code.
				s.emit(ErrorMsg{
					CmdID:     CmdID(res.cmdID),
					Err:       fmt.Errorf("%w: %v", ErrPanicRecovered, res.panicErr),
					Stack:     res.stack,
					Submitted: res.submitted,
					Priority:  res.priority,
					Frame:     s.frame,
				})
			}
		}
	}
}

// runFrame executes one render frame. See spec-1.4 § 3.4 frame
// lifecycle invariants for the full 8-step happy path and the 6
// exception-path × ownership-correctness proofs.
//
// spec-1.4 R2 CRIT-B: lastFrame ownership transfer happens exactly
// once, at the very end. applyPatches NEVER mutates s.lastFrame.
func (s *FrameScheduler) runFrame(_ context.Context) {
	start := time.Now()
	s.frame++

	// Step 1: snapshot prev to local var.
	prev := s.lastFrame
	var next *buffer.Grid
	adopted := false

	// Step 2: defer cleanup — catastrophic recover + ownership safety
	// (spec-1.4 R2 H-2). If we panic before adopted=true, Release any
	// acquired next. prev is unaffected: it stays referenced by
	// s.lastFrame so the next frame can retry diff against the same
	// state. spec-1.4 R3 M-C: capture debug.Stack() once and reuse.
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			slog.Error("frame panic recovered",
				"panic", r,
				"frame", s.frame,
				"adopted", adopted,
				"stack", string(stack))
			s.emit(ErrorMsg{
				CmdID:    0, // frame-level panic has no Cmd identity
				Err:      fmt.Errorf("%w: frame body: %v", ErrPanicRecovered, r),
				Stack:    stack,
				Priority: PriorityNormal,
				Frame:    s.frame,
			})
		}
		if !adopted && next != nil {
			s.pool.Release(next)
		}
	}()

	// Step 3: drain queue → submit to workers (non-blocking).
	// spec-1.4 R3 user 决策 Option B: never block the frame loop on a
	// full worker channel. popIf + TrySubmit is atomic with respect to
	// concurrent ScheduleAt calls: on submit failure the item remains at
	// the queue head for the next frame; on success the exact submitted
	// item is removed.
	for s.queue.popIf(s.workers.TrySubmit) {
	}

	// Step 4: acquire next.
	cols, rows := s.driver.Size()
	var err error
	next, err = s.pool.Acquire(cols, rows)
	if err != nil {
		slog.Warn("BufferPool.Acquire failed",
			"err", err, "cols", cols, "rows", rows, "frame", s.frame)
		return
	}

	// Step 5: RenderFn fills next (main-loop-only contract).
	s.render(next)

	// Step 6: diff + apply.
	// Wrap prev in a Buffer interface only when non-nil; passing a typed
	// nil *buffer.Grid through the interface would defeat spec-1.3 R-10
	// "prev==nil ⇒ fullRedraw" detection (Go interface-nil gotcha).
	var prevBuf buffer.Buffer
	if prev != nil {
		prevBuf = prev
	}
	patches := s.diff.Diff(prevBuf, next)
	_ = s.applyPatches(patches) // sawResize ignored — currently informational only

	// Step 7: Show + release prev.
	s.driver.Show()
	if prev != nil {
		s.pool.Release(prev)
	}

	// Step 8: adopt next as new lastFrame.
	s.lastFrame = next
	adopted = true

	// tick non-blocking.
	select {
	case s.tickCh <- Tick{When: time.Now(), Frame: s.frame}:
	default:
	}

	if elapsed := time.Since(start); elapsed > s.frameDeadline {
		slog.Warn("frame budget overshoot",
			"elapsed", elapsed, "deadline", s.frameDeadline, "frame", s.frame)
	}
}

// applyPatches translates optimizer.Patch into Driver.SetCell /
// Driver.Resize calls. The returned sawResize is true if at least one
// PatchResize was applied this frame; callers may use it for logging
// or metrics, but the next-frame fullRedraw is automatic via the
// optimizer detecting size mismatch on the new lastFrame (spec-1.4 R3
// H-D + R2.1: sawResize naming, not needsFullRedraw — the resize is
// already fully applied in this frame's patch stream).
//
// spec-1.3 § 10 forward contract: Cell{} (Ch=0) must clear the cell —
// adapter writes space ' ' since tcell has no well-defined Ch=0
// behavior. Single translation point (spec-1.4 D-4).
//
// spec-1.4 R2 CRIT-B: this function MUST NOT mutate s.lastFrame
// ownership; PatchResize is applied to the driver only.
func (s *FrameScheduler) applyPatches(patches []optimizer.Patch) (sawResize bool) {
	for _, p := range patches {
		switch p.Kind {
		case optimizer.PatchSetCell:
			ch, st := p.Cell.Ch, p.Cell.St
			if ch == 0 {
				// spec-1.3 § 10 Cell{} clear translation
				ch, st = ' ', style.Style{}
			}
			s.driver.SetCell(p.X, p.Y, ch, st)
		case optimizer.PatchResize:
			s.driver.Resize(p.NewCols, p.NewRows)
			sawResize = true
		}
	}
	return sawResize
}
