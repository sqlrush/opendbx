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

// Msg is the type alias for any message flowing through the scheduler
// dispatch path (priorityMsgCh, results, hook).
//
// spec-1.4 R3.1 errata (spec-1.15 R3 codex CRIT-3 driven): originally a
// marker `interface{ msg() }` with an UNEXPORTED method, which made it
// impossible for cross-package callers (e.g. the program package) to
// implement the marker. `type Msg = any` is a Go type alias to the empty
// interface; any type satisfies it without ceremony, and program-side
// KeyMsg / ResizeMsg / QuitMsg can flow through without needing a
// marker method.
//
// ErrorMsg (below) is a concrete struct in the scheduler package and is
// trivially a Msg via this alias; the H-6 debug-context fields (CmdID,
// Submitted, Priority, Frame) are preserved.
type Msg = any

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

// (R3.1: Msg is `any`, so ErrorMsg satisfies Msg without a marker method.
// The original ErrorMsg.msg() method is removed; existing `errors.Is`
// + ErrorMsg field access remains the canonical API.)

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
	priorityMsgCh chan Msg    // R3.5: UI/system Msg direct path (bypasses worker pool)
	msgHook       func(Msg)   // R3.3: optional main-loop dispatch hook
	stopped       atomic.Bool // R3.5: Stop / Run-exit guard; EmitMsg drops + warns when set
	lastFrame     *buffer.Grid
	frame         int
	nextCmdID     atomic.Uint64
}

// Option configures a FrameScheduler at construction.
//
// R3.3 / R3.5: WithMsgHook installs a main-loop dispatch hook (program
// package uses this to take ownership of Msg routing on the scheduler
// goroutine; old callers without a hook fall back to msgCh).
type Option func(*FrameScheduler)

// WithMsgHook installs a main-loop msg dispatch hook. When set, the
// scheduler invokes hook(msg) synchronously on the main render goroutine
// for every Msg drained from priorityMsgCh / results — Msgs do not go
// through the msgCh fallback. Caller MUST NOT block in hook (it runs
// inside the frame loop). nil hook (default) preserves the original
// msgCh-only behaviour.
func WithMsgHook(hook func(Msg)) Option {
	return func(s *FrameScheduler) { s.msgHook = hook }
}

// Compile-time assertion that *FrameScheduler satisfies Scheduler.
var _ Scheduler = (*FrameScheduler)(nil)

// NewFrameScheduler constructs a FrameScheduler.
//
// driver MUST be non-nil. render MUST be non-nil (spec-1.4 R2 CRIT-A
// — silently rendering blank frames hides a caller wiring bug; we
// panic at construction time with a clear message instead). fps <= 0
// is clamped to 60.
func NewFrameScheduler(driver terminal.Driver, fps int, render RenderFn, opts ...Option) *FrameScheduler {
	if driver == nil {
		panic("scheduler.NewFrameScheduler: driver must be non-nil (caller lifecycle bug)")
	}
	if render == nil {
		panic("scheduler.NewFrameScheduler: render must be non-nil — spec-1.4 R2 CRIT-A; pass a RenderFn closure that populates the next *buffer.Grid from your model state")
	}
	if fps <= 0 {
		fps = 60
	}
	s := &FrameScheduler{
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
		// R3.5: priorityMsgCh is buffered to 256 (matches the spec budget)
		// and is NEVER closed — Stop sets the stopped flag and EmitMsg
		// drops new sends to avoid send-on-closed panic.
		priorityMsgCh: make(chan Msg, 256),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
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
//
// R3.3/R3.5 behaviour: when WithMsgHook is installed, the hook receives
// every Msg synchronously on the main goroutine and msgCh receives
// nothing. Without a hook, the scheduler falls back to non-blocking
// emit on msgCh for backwards compatibility.
func (s *FrameScheduler) Msgs() <-chan Msg { return s.msgCh }

// CurrentFrame returns the current frame counter (spec-1.4 R3.4 public).
//
// Concurrency: the field is mutated only on the render goroutine. The
// intended callers are msgHook implementations (which run synchronously
// inside the render goroutine via WithMsgHook), so reads are racy-only
// when invoked from arbitrary external goroutines. Cross-goroutine
// callers MUST treat the return value as a best-effort snapshot used
// only for ErrorMsg debug context (the exact value is not load-bearing).
func (s *FrameScheduler) CurrentFrame() int {
	return s.frame
}

// EmitMsg posts msg directly onto the priority message queue, bypassing
// the worker pool. spec-1.4 R3.5 public API for UI/system messages
// (KeyMsg / ResizeMsg / Ctrl+C / quit-disarm) that MUST NOT be delayed
// by long-running Cmds in the worker pool.
//
// Non-blocking by design: if priorityMsgCh is full, EmitMsg drops one
// oldest message and retries; if even that fails it logs and drops the
// new message (same policy as emit()). Safe to call after Stop —
// stopped flag short-circuits and logs a warning to avoid send-on-
// closed panic (priorityMsgCh is NEVER closed).
func (s *FrameScheduler) EmitMsg(msg Msg) {
	if s.stopped.Load() {
		slog.Warn("scheduler.EmitMsg called after Stop; dropping message", "msg_type", fmt.Sprintf("%T", msg))
		return
	}
	select {
	case s.priorityMsgCh <- msg:
		return
	default:
	}
	// drop one oldest, retry
	select {
	case <-s.priorityMsgCh:
	default:
	}
	select {
	case s.priorityMsgCh <- msg:
	default:
		slog.Warn("scheduler.priorityMsgCh saturated; dropping message", "msg_type", fmt.Sprintf("%T", msg))
	}
}

// EmitError is a typed wrapper over EmitMsg for ErrorMsg values.
// spec-1.4 R3.4 + R3.5: caller constructs the full ErrorMsg (CmdID /
// Submitted / Priority / Frame H-6 context) and submits via this entry
// to keep the wrapper-as-typed-API contract clear.
//
// Bare `error` is intentionally NOT accepted — callers MUST build the
// ErrorMsg with full context so debug telemetry doesn't lose CmdID /
// Submitted / Priority / Frame fields.
func (s *FrameScheduler) EmitError(msg ErrorMsg) {
	s.EmitMsg(msg)
}

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

// maxPriorityMsgsPerFrame caps how many priorityMsgCh entries are
// drained per frame. Bounded so a paste/key flood cannot starve render
// and worker-results dispatch forever (spec-1.4 R3.5 + spec-1.15 R5
// drain policy).
const maxPriorityMsgsPerFrame = 64

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
	// R3.5: set stopped flag BEFORE close(msgCh) so any racing EmitMsg
	// caller observes stopped=true and bails out via the drop+warn path.
	// priorityMsgCh itself is intentionally NEVER closed to avoid
	// send-on-closed panics for in-flight goroutines (timer callbacks).
	defer s.stopped.Store(true)
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
			s.drainPriorityMsgs()
			s.drainResults()
			s.runFrame(ctx)
		case res, ok := <-s.workers.Results():
			if !ok {
				continue
			}
			s.dispatchResult(res)
		case m := <-s.priorityMsgCh:
			// R3.5: priority Msg ticked outside frame boundary — dispatch
			// immediately for low-latency UI response (still single-
			// goroutine; no race with renderFn since this select arm is
			// mutually exclusive with the ticker arm).
			s.dispatchMsg(m)
		}
	}
}

// drainPriorityMsgs drains up to maxPriorityMsgsPerFrame entries off
// priorityMsgCh and dispatches them synchronously. Called at the top of
// each ticker frame (R3.5 + spec-1.15 R5: priority first, bounded).
func (s *FrameScheduler) drainPriorityMsgs() {
	for i := 0; i < maxPriorityMsgsPerFrame; i++ {
		select {
		case m := <-s.priorityMsgCh:
			s.dispatchMsg(m)
		default:
			return
		}
	}
}

// drainResults drains pending worker results (success Msg or panic
// ErrorMsg) and dispatches them. Called after drainPriorityMsgs so
// system Msgs always have priority.
func (s *FrameScheduler) drainResults() {
	for {
		select {
		case res, ok := <-s.workers.Results():
			if !ok {
				return
			}
			s.dispatchResult(res)
		default:
			return
		}
	}
}

// dispatchResult converts a workerResult into the appropriate Msg
// (success Msg via R3.2, or ErrorMsg via H-6) and dispatches.
func (s *FrameScheduler) dispatchResult(res workerResult) {
	if res.panicErr != nil {
		// spec-1.4 R3 H-A: wrap with %w so callers can use
		// errors.Is(em.Err, ErrPanicRecovered) to programmatically detect
		// scheduler panic-recovered errors (rule 7 errcode chain).
		s.dispatchMsg(ErrorMsg{
			CmdID:     CmdID(res.cmdID),
			Err:       fmt.Errorf("%w: %v", ErrPanicRecovered, res.panicErr),
			Stack:     res.stack,
			Submitted: res.submitted,
			Priority:  res.priority,
			Frame:     s.frame,
		})
		return
	}
	// R3.2: success Msg dispatch
	if res.successMsg != nil {
		s.dispatchMsg(res.successMsg)
	}
}

// dispatchMsg sends m to the installed msgHook (R3.3) or, when no hook
// is configured, to the legacy msgCh via non-blocking emit (R3.5: never
// raw blocking send).
func (s *FrameScheduler) dispatchMsg(m Msg) {
	if s.msgHook != nil {
		s.msgHook(m)
		return
	}
	s.emit(m)
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
