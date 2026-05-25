// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package program

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/sqlrush/opendbx/internal/app/cli/input"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/scheduler"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

// DefaultQuitWindow is the spec-1.15 R3 default Ctrl+C double-press
// window — CC parity per useDoublePress.ts:6 (DOUBLE_PRESS_TIMEOUT_MS).
const DefaultQuitWindow = 800 * time.Millisecond

// Program is the spec-1.15 application-layer skeleton. See package doc
// for the single-goroutine ownership rationale.
type Program struct {
	driver      terminal.Driver
	scheduler   *scheduler.FrameScheduler
	model       Model     // single-goroutine: only handleMsg writes, only renderFn reads (same goroutine)
	quitArmed   bool      // see above
	quitArmedAt time.Time // R3 MED-2: clock.Now() of arm; quit decision verifies window via time, not just flag
	quitTimer   Timer
	quitWindow  time.Duration
	clock       SimClock
	cancelCause func()    // injected by Run to break out of the scheduler loop on QuitMsg
	runOnce     sync.Once // R2 L-4: single-use Run guard; second concurrent call panics
}

// Option configures a Program at construction.
type Option func(*Program)

// WithQuitWindow overrides the default 800ms Ctrl+C double-press
// window. Tests use a 10-minute value to capture the quit-armed state
// deterministically.
func WithQuitWindow(d time.Duration) Option {
	return func(p *Program) { p.quitWindow = d }
}

// WithClock injects a fake clock (SimClock) for deterministic timer
// behaviour in tests. Production callers do not need this — the
// realClock default wraps the stdlib time package.
func WithClock(c SimClock) Option {
	return func(p *Program) { p.clock = c }
}

// New constructs a Program. driver is the terminal.Driver shared with
// the scheduler; initial is the starting Model; opts apply functional
// options (WithQuitWindow / WithClock).
func New(driver terminal.Driver, initial Model, opts ...Option) *Program {
	if driver == nil {
		panic("program.New: driver must be non-nil")
	}
	if initial == nil {
		panic("program.New: initial Model must be non-nil")
	}
	p := &Program{
		driver:     driver,
		model:      initial,
		quitWindow: DefaultQuitWindow,
		clock:      realClock{},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Run drives the TUI to completion. Returns the underlying
// scheduler.Run error (context.Canceled / DeadlineExceeded on graceful
// shutdown, or scheduler/driver init errors on hard failure).
//
// spec-1.15 R3 / R4: scheduler.Run is the only main loop. Program
// installs WithMsgHook(handleMsg) so dispatch happens on the scheduler
// goroutine. PollEvent goroutine is the only auxiliary goroutine; it
// uses driver.PollEvent (blocking, ctx-aware) and routes events via
// scheduler.EmitMsg (priority queue) so worker pool saturation cannot
// stall input.
func (p *Program) Run(ctx context.Context) error {
	// R2 L-4: single-use guard. Concurrent Run() would race on p.scheduler
	// assignment and break the single-goroutine ownership contract.
	firstCall := false
	p.runOnce.Do(func() { firstCall = true })
	if !firstCall {
		panic("program.Run: already running (single-use; not safe for concurrent or repeat invocation)")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	p.cancelCause = cancel
	defer func() { p.cancelCause = nil }()

	// spec-1.15 R3 CRIT-2: scheduler owns the only main loop.
	p.scheduler = scheduler.NewFrameScheduler(p.driver, 60, p.renderFn,
		scheduler.WithMsgHook(p.handleMsg),
	)

	defer p.runCleanup()

	// spec-1.15 R3 init Cmd → scheduler.Schedule (real async work; nil-tolerant).
	if cmd := p.model.Init(); cmd != nil {
		p.scheduler.Schedule(cmd)
	}

	// PollEvent goroutine — converts terminal events to KeyMsg/ResizeMsg
	// and posts them via EmitMsg (R3.5 priority queue). Exits when
	// driver.PollEvent returns an error / ctx.Done.
	go p.pollEvents(ctx)

	// errcode-lint:exempt -- spec-1.15 D-1: scheduler.Run returns context.Canceled / DeadlineExceeded verbatim by design (stdlib sentinel pass-through, matches spec-0.12 D-3 tui.Run + spec-1.4 D-1 pattern); driver.Init errors are pre-wrapped at the terminal layer.
	return p.scheduler.Run(ctx)
}

// pollEvents blocks on driver.PollEvent and routes each event into the
// scheduler's priority message queue. Runs in its own goroutine; does
// NOT access p.model (model touch only happens in handleMsg / renderFn
// on the scheduler goroutine).
//
// R3 codex HIGH-1 fix: wait for scheduler.Started() before issuing the
// first PollEvent — driver.Init runs on the scheduler goroutine inside
// scheduler.Run, and pollEvents must not call driver methods before
// that completes.
func (p *Program) pollEvents(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-p.scheduler.Started():
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		ev, err := p.driver.PollEvent(ctx)
		if err != nil {
			return
		}
		if ev == nil {
			return
		}
		switch e := ev.(type) {
		case terminal.EventKey:
			p.scheduler.EmitMsg(KeyMsg{Code: e.Code, Rune: e.Rune, Mod: e.ShiftCtrlAlt})
		case terminal.EventResize:
			p.scheduler.EmitMsg(ResizeMsg{Cols: e.Cols, Rows: e.Rows})
		case terminal.EventInterrupt:
			// scheduler / external interrupt — drop; ctx.Done will fire
			// the actual cancellation path.
		}
	}
}

// handleMsg is the WithMsgHook callback. Invoked synchronously on the
// scheduler goroutine for every Msg drained from priorityMsgCh or
// worker results. Single-goroutine ownership of p.model — no atomic
// or mutex needed.
func (p *Program) handleMsg(msg scheduler.Msg) {
	defer func() {
		if r := recover(); r != nil {
			// R2 H-1 (claude code-reviewer + go-reviewer 共识):
			// - wrap with %w: scheduler.ErrPanicRecovered so callers can
			//   errors.Is(em.Err, scheduler.ErrPanicRecovered) parity with
			//   spec-1.4 R3 H-A worker pool panic path
			// - capture debug.Stack() into ErrorMsg.Stack (H-6 contract)
			p.scheduler.EmitError(scheduler.ErrorMsg{
				Err:       fmt.Errorf("%w: program.handleMsg: %v", scheduler.ErrPanicRecovered, r),
				Stack:     debug.Stack(),
				Submitted: p.clock.Now(),
				Priority:  scheduler.PriorityNormal, // R3 codex LOW-1: explicit; zero value PriorityHigh would mislabel recover
				Frame:     p.scheduler.CurrentFrame(),
			})
		}
	}()

	if p.preDispatchSystem(msg) {
		return
	}

	newModel, cmd := p.model.Update(msg)
	if newModel != nil {
		p.model = newModel
	}
	if cmd != nil {
		p.scheduler.Schedule(cmd)
	}
}

// preDispatchSystem handles system-level Msgs before they reach the
// Model. Returns true when the Msg is fully handled at the program
// level and should NOT be forwarded to Model.Update.
func (p *Program) preDispatchSystem(msg scheduler.Msg) (handled bool) {
	switch m := msg.(type) {
	case KeyMsg:
		// Ctrl+C is the only system-level key in spec-1.15 baseline.
		// Other keys (Esc / printable / Enter) are forwarded to Model.
		if m.Code == terminal.KeyCtrlC {
			if p.handleCtrlC() {
				p.requestQuit()
			} else {
				// First press within window — also dispatch CancelCmdMsg
				// so the Model can cancel any in-flight context.
				// R2 H-2: must NOT discard newModel; immutable Update
				// contract requires replacing p.model when Update returns
				// a non-nil new model (rule 12 immutable data).
				newModel, cmd := p.model.Update(CancelCmdMsg{})
				if newModel != nil {
					p.model = newModel
				}
				if cmd != nil {
					p.scheduler.Schedule(cmd)
				}
			}
			return true
		}
		return false
	case QuitMsg:
		p.requestQuit()
		return true
	case quitDisarmMsg:
		p.quitArmed = false
		return true
	}
	return false
}

// handleCtrlC implements the spec-1.15 D-5 double-press protocol.
// Returns true when this press completes the double-press (quit).
//
// R3 codex MED-2 fix: decision is time-based (clock.Now() - quitArmedAt
// <= quitWindow), not flag-only. quitDisarmMsg delivery delays (worker
// pool saturation / priorityMsgCh drop-oldest) would otherwise let a
// second Ctrl+C arriving > 800ms after the first still quit because
// the disarmMsg hadn't been processed yet. The timer-driven disarm
// still runs (clears the "Press Ctrl+C again to quit" footer) but is
// no longer load-bearing for correctness.
func (p *Program) handleCtrlC() (quit bool) {
	now := p.clock.Now()
	if p.quitArmed && now.Sub(p.quitArmedAt) <= p.quitWindow {
		return true
	}
	p.quitArmed = true
	p.quitArmedAt = now
	if p.quitTimer != nil {
		p.quitTimer.Stop()
	}
	// Timer goroutine cannot write p.quitArmed directly (cross-goroutine
	// race). It posts quitDisarmMsg via EmitMsg, which the main loop
	// drains on the next frame and processes in preDispatchSystem.
	p.quitTimer = p.clock.AfterFunc(p.quitWindow, func() {
		p.scheduler.EmitMsg(quitDisarmMsg{})
	})
	return false
}

// requestQuit cancels the Run context, which propagates through
// scheduler.Run → ctx.Err() return.
func (p *Program) requestQuit() {
	if p.cancelCause != nil {
		p.cancelCause()
	}
}

// renderFn is the scheduler RenderFn callback. Runs on the scheduler
// goroutine alongside handleMsg — single-goroutine ownership of
// p.model holds, no race.
func (p *Program) renderFn(next *buffer.Grid) {
	cols, rows := next.Size()
	layout := Layout{Cols: cols, Rows: rows}
	sbCols, sbRows := layout.ScrollbackSize()
	if sbRows > 0 {
		sbBuf := p.model.View(sbCols, sbRows)
		paintBufferAt(next, sbBuf, 0, 0)
	}
	if r := layout.InputRow(); r >= 0 {
		p.paintInputRow(next, r)
	}
	if r := layout.StatusLine(); r >= 0 {
		p.paintStatusLine(next, r)
	}
}

// paintInputRow renders the input row at the given grid row.
//
// spec-1.16 R2 H-3 ★A: mode classification by input.DeriveMode(buffer)
// at read site (mode is NEVER state). Natural mode renders "> {buf}_";
// Slash/SQL mode renders "{buf}_" with Buffer[0] (the trigger char)
// acting as the mode glyph itself (no "> " prefix — CC `!bash` parity).
// quitArmed override remains (spec-1.15 D-5; R2 L-2 explicit).
func (p *Program) paintInputRow(grid *buffer.Grid, row int) {
	cols, _ := grid.Size()
	if p.quitArmed {
		s := "Press Ctrl+C again to quit"
		paintTextAt(grid, s, 0, row, style.Style{Bold: true}, cols)
		return
	}
	im, hasInput := p.model.(InputModel)
	if !hasInput {
		paintTextAt(grid, "> ", 0, row, style.Style{}, cols)
		return
	}
	st := im.InputState()
	mode := input.DeriveMode(st.Buffer)
	st_in := input.StyleFor(mode)
	if mode == input.InputModeNatural {
		// Natural mode keeps the "> " prompt.
		x := paintTextAt(grid, "> ", 0, row, style.Style{}, cols)
		paintTextAt(grid, st.Buffer, x, row, st_in, cols)
	} else {
		// Slash/SQL: Buffer literal is itself the mode glyph + body.
		paintTextAt(grid, st.Buffer, 0, row, st_in, cols)
	}
}

// paintStatusLine renders the status line. Mode segment is unconditionally
// appended at the end (spec-1.16 D-5 + R2 M-4 / R-9 dedup policy:
// StatusSegmenter implementors MUST NOT include their own mode segment
// to avoid duplication; this is a breaking constraint introduced by
// spec-1.16).
func (p *Program) paintStatusLine(grid *buffer.Grid, row int) {
	cols, _ := grid.Size()
	var segs []StatusSegment
	if ss, ok := p.model.(StatusSegmenter); ok {
		segs = ss.StatusSegments()
	}
	if len(segs) == 0 {
		segs = []StatusSegment{{Text: "opendbx"}}
	}
	// Append mode segment from InputModel.Buffer (spec-1.16 D-5).
	if im, ok := p.model.(InputModel); ok {
		mode := input.DeriveMode(im.InputState().Buffer)
		segs = append(segs, StatusSegment{Text: mode.String()})
	}
	x := 0
	for _, seg := range segs {
		if x >= cols {
			break
		}
		x = paintTextAt(grid, seg.Text, x, row, seg.Style, cols)
		// segment separator
		if x < cols {
			grid.SetCell(x, row, buffer.Cell{Ch: ' '})
			x++
		}
	}
}

// runCleanup invokes Model.Cleanup() synchronously at Run exit. Run
// happens AFTER scheduler.Run returns (so the worker pool is already
// stopped) — Cleanup callbacks must therefore not depend on scheduler /
// worker availability.
//
// R2 M-3 (go-reviewer): defer/recover lives in runCleanupCmd helper to
// keep the recovery scope flat (rule 12 nesting ≤ 4 + DRY).
func (p *Program) runCleanup() {
	c, ok := p.model.(Cleanup)
	if !ok {
		return
	}
	cmd := c.Cleanup()
	if cmd == nil {
		return
	}
	runCleanupCmd(cmd)
}

// runCleanupCmd executes a Cleanup Cmd synchronously with a panic
// recovery scope. Logs but does not propagate panics — Run is exiting
// and there is no scheduler available to route an ErrorMsg through.
func runCleanupCmd(cmd scheduler.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("program: Cleanup Cmd panicked", "panic", r)
		}
	}()
	_ = cmd()
}

// paintBufferAt copies cells from src into dst starting at (xOff, yOff).
// Out-of-bounds cells are dropped silently. nil src is a no-op.
func paintBufferAt(dst *buffer.Grid, src buffer.Buffer, xOff, yOff int) {
	if src == nil {
		return
	}
	dstCols, dstRows := dst.Size()
	srcCols, srcRows := src.Size()
	for y := 0; y < srcRows; y++ {
		dy := yOff + y
		if dy < 0 || dy >= dstRows {
			continue
		}
		for x := 0; x < srcCols; x++ {
			dx := xOff + x
			if dx < 0 || dx >= dstCols {
				continue
			}
			dst.SetCell(dx, dy, src.Cell(x, y))
		}
	}
}

// paintTextAt writes runes from s into grid starting at (x, y), using
// style st. Stops at cols. Returns the next free x position. Wide
// runes (RuneWidth == 2) consume 2 cells; SetCell auto-writes the
// continuation cell.
func paintTextAt(grid *buffer.Grid, s string, x, y int, st style.Style, cols int) int {
	for _, r := range s {
		rw := width.RuneWidth(r)
		if rw <= 0 {
			continue
		}
		if x+rw > cols {
			break
		}
		grid.SetCell(x, y, buffer.Cell{Ch: r, St: st})
		x += rw
	}
	return x
}
