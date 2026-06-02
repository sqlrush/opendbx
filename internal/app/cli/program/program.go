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
	"sync/atomic"
	"time"

	"github.com/sqlrush/opendbx/internal/app/cli/input"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/paint"
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

	// schedSet is closed by Run immediately after p.scheduler is
	// assigned, so test seams (WaitStartedForTest) can read p.scheduler
	// race-free (spec-1.17 D-7 integration harness). Created in New.
	schedSet chan struct{}

	// msgCount counts Msgs processed by handleMsg. Atomic so the spec-1.17
	// R-fix MED-7 integration harness can deterministically wait for an
	// injected key to be drained (instead of time.Sleep) without racing on
	// p.model. Incremented on the scheduler goroutine, read from the test
	// goroutine.
	msgCount atomic.Int64
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
		schedSet:   make(chan struct{}),
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
	// spec-1.17 D-7: signal p.scheduler is assigned so test seams can
	// read it race-free. Production has no observer; close is cheap.
	close(p.schedSet)

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
	// spec-1.17 R-fix MED-7: count every drained Msg so the integration
	// harness can deterministically wait for an injected key to be
	// processed (atomic; read from the test goroutine via
	// ProcessedMsgCountForTest). Counted before dispatch so even
	// system-intercepted Msgs (Ctrl+C / Ctrl+\) increment.
	p.msgCount.Add(1)
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

	// spec-1.17 R2 D-5: decode KeyMsg → keybindings.Action once at the
	// program layer and forward as KeyActionMsg. Models switch on
	// Action; raw KeyMsg.Code remains available via msg.Key for
	// fallback / log. Non-KeyMsg messages pass through unchanged.
	if k, ok := msg.(KeyMsg); ok {
		msg = KeyActionMsg{Key: k, Action: decodeKeyMsg(k)}
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
		// Ctrl+\ (KeyCtrlBackslash) is an immediate hard-exit (spec-0.12
		// contract; spec-1.17 R-fix HIGH-1). It bypasses the double-press
		// window — a single press quits. Intercepted here at the system
		// level so it never reaches the Model (parity with the retired
		// tui.Run path which exited on Ctrl+C OR Ctrl+\).
		if m.Code == terminal.KeyCtrlBackslash {
			p.requestQuit()
			return true
		}
		// Ctrl+C is the double-press quit key (spec-1.15 D-5).
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
		paint.BlitAt(next, sbBuf, 0, 0)
	}
	// R4 L-2 go-reviewer: extract InputState once per frame so input row
	// + status line share an identical snapshot. Removes the burden on
	// InputModel implementors to guarantee InputState() returns the same
	// value across repeated calls (spec-1.16 R2 M-7 contract still
	// documented in model.go for callers that bypass renderFn).
	var (
		im     InputModel
		state  InputState
		hasInp bool
	)
	if v, ok := p.model.(InputModel); ok {
		im, hasInp = v, true
		state = v.InputState()
	}
	p.paintInputBox(next, layout, im, state, hasInp)
	if r := layout.StatusLine(); r >= 0 {
		p.paintStatusLine(next, r, im, state, hasInp)
	}
}

// inputBoxHint is the CC borderText hint embedded in the bottom rule
// (CC PromptInputFooterLeftSide.tsx:411 "? for shortcuts"). spec-1.25 D-3.
const inputBoxHint = "? for shortcuts"

// inputBoxRune is the horizontal rule rune for the top/bottom input
// borders. spec-1.25 D-3: CC PromptInput.tsx:2268 uses borderStyle="round"
// with borderLeft/Right=false — i.e. top + bottom rules, no side walls.
const inputBoxRule = '─'

// paintInputBox renders the spec-1.25 D-3 input region. In box mode
// (Layout.BoxMode) it draws a top rule, the content row (reusing
// paintInputRow so spec-1.16 mode/cursor/placeholder logic is unchanged),
// and a bottom rule carrying the CC borderText hint. Below the N=5 floor
// it degrades to a single content row (spec-1.16 parity). Painting nothing
// when the content row is absent (tiny terminal).
func (p *Program) paintInputBox(grid *buffer.Grid, layout Layout, im InputModel, state InputState, hasInput bool) {
	content := layout.InputRow()
	if content < 0 {
		return
	}
	if !layout.BoxMode() {
		p.paintInputRow(grid, content, im, state, hasInput)
		return
	}
	cols, _ := grid.Size()
	dim := style.Style{FG: style.Palette(8)} // terminal-relative grey (R-10)
	if top := layout.InputBoxTop(); top >= 0 {
		paintInputRule(grid, top, "", dim, cols)
	}
	p.paintInputRow(grid, content, im, state, hasInput)
	if bottom := layout.InputBoxBottom(); bottom >= 0 {
		paintInputRule(grid, bottom, inputBoxHint, dim, cols)
	}
}

// paintInputRule fills row y with the horizontal rule rune across cols
// and, when hint is non-empty, overlays " <hint> " near the right end
// (CC borderText). The surrounding spaces punch a gap in the rule so the
// hint reads as "──── ? for shortcuts ─". Rule-only when too narrow for
// the hint. spec-1.25 D-3.
func paintInputRule(grid *buffer.Grid, y int, hint string, st style.Style, cols int) {
	if cols <= 0 || y < 0 {
		return
	}
	for x := 0; x < cols; x++ {
		grid.SetCell(x, y, buffer.Cell{Ch: inputBoxRule, St: st})
	}
	if hint == "" {
		return
	}
	label := " " + hint + " "
	start := cols - width.Width(label) - 1 // keep 1 trailing rule cell
	if start < 0 {
		return // too narrow: rule only
	}
	paintTextAt(grid, label, start, y, st, cols)
}

// paintInputRow renders the input row at the given grid row.
//
// spec-1.16 R2 H-3 ★A: mode classification by input.DeriveMode(buffer)
// at read site (mode is NEVER state). Natural mode renders "> {buf}_";
// Slash/SQL mode renders "{buf}_" with Buffer[0] (the trigger char)
// acting as the mode glyph itself (no "> " prefix — CC `!bash` parity).
// quitArmed override remains (spec-1.15 D-5; R2 L-2 explicit).
//
// R4 H-1 fix: read InputState.Cursor to position the '_' cursor glyph.
// spec-1.16 H-7 scope-limit keeps Cursor at end of buffer (rune count);
// paintInputRow places the glyph at the rune-position-relative cell.
// R4 L-2: receives extracted im / state from renderFn for cross-paint
// consistency (single read per frame).
func (p *Program) paintInputRow(grid *buffer.Grid, row int, _ InputModel, state InputState, hasInput bool) {
	cols, _ := grid.Size()
	if p.quitArmed {
		s := "Press Ctrl+C again to quit"
		paintTextAt(grid, s, 0, row, style.Style{Bold: true}, cols)
		return
	}
	if !hasInput {
		paintTextAt(grid, "> _", 0, row, style.Style{}, cols)
		return
	}
	mode := input.DeriveMode(state.Buffer)
	inputStyle := input.StyleFor(mode)
	// R3 codex MED-1 fix: split buffer at Cursor rune position so the
	// '_' glyph reflects InputState.Cursor (no longer dead field).
	// Buffer is sliced as runes (Cursor is rune-position per spec D-1),
	// and the pre/post halves are painted separately with the cursor
	// glyph between them. spec-1.16 H-7 scope-limit keeps Cursor at
	// len(runes(Buffer)); this rendering is forward-compatible with
	// spec-1.17 mid-cursor edits.
	runes := []rune(state.Buffer)
	cursor := state.Cursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	pre := string(runes[:cursor])
	post := string(runes[cursor:])
	var x int
	if mode == input.ModeNatural {
		// Natural mode keeps the "> " prompt.
		x = paintTextAt(grid, "> ", 0, row, style.Style{}, cols)
	}
	x = paintTextAt(grid, pre, x, row, inputStyle, cols)
	if x < cols {
		grid.SetCell(x, row, buffer.Cell{Ch: '_', St: style.Style{Bold: true}})
		x++
	}
	paintTextAt(grid, post, x, row, inputStyle, cols)
}

// paintStatusLine renders the status line. The mode segment is
// **unconditionally appended** at the end (append-only contract, not a
// dedup implementation per codex R3 LOW): the InputModel-derived mode
// segment is added even if StatusSegmenter already returned a segment
// with the same text. StatusSegmenter implementors are advised not to
// emit their own mode segment to avoid visual duplication (spec-1.16
// D-5 + R-9). R4 L-2: state arg comes from renderFn-extracted snapshot.
func (p *Program) paintStatusLine(grid *buffer.Grid, row int, _ InputModel, state InputState, hasInput bool) {
	cols, _ := grid.Size()
	var segs []StatusSegment
	if ss, ok := p.model.(StatusSegmenter); ok {
		segs = ss.StatusSegments()
	}
	if len(segs) == 0 {
		segs = []StatusSegment{{Text: "opendbx"}}
	}
	// Append mode segment from InputModel.Buffer (spec-1.16 D-5).
	if hasInput {
		mode := input.DeriveMode(state.Buffer)
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
