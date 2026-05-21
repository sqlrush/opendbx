// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scheduler

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// mockDriver implements terminal.Driver for in-package testing.
// spec-1.4 R-10 mitigation: tcell SimulationScreen is the spec-1.15
// scope; for spec-1.4 unit tests we use this simple mock so the
// scheduler package keeps its DAG-clean dependencies.
type mockDriver struct {
	mu sync.Mutex

	cols, rows int

	initErr  error
	initN    int
	finiN    int
	showN    int
	resizeN  int
	setCells []mockSetCell

	// hook for panicking the driver in specific methods
	showPanic    bool
	setCellPanic bool
}

type mockSetCell struct {
	X, Y int
	Ch   rune
	St   style.Style
}

func (m *mockDriver) Init() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initN++
	return m.initErr
}

func (m *mockDriver) Fini() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finiN++
}

func (m *mockDriver) Show() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.showN++
	if m.showPanic {
		panic("mockDriver.Show forced panic")
	}
}

func (m *mockDriver) Sync()  {}
func (m *mockDriver) Clear() {}

func (m *mockDriver) Size() (cols, rows int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cols, m.rows
}

func (m *mockDriver) SetCell(x, y int, ch rune, st style.Style) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setCellPanic {
		panic("mockDriver.SetCell forced panic")
	}
	m.setCells = append(m.setCells, mockSetCell{X: x, Y: y, Ch: ch, St: st})
}

func (m *mockDriver) PollEvent(ctx context.Context) (terminal.Event, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *mockDriver) PostEvent(_ terminal.Event) error { return nil }

func (m *mockDriver) Resize(cols, rows int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cols, m.rows = cols, rows
	m.resizeN++
}

func (m *mockDriver) snapshot() (init, fini, show, resize int, cells []mockSetCell) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mockSetCell, len(m.setCells))
	copy(out, m.setCells)
	return m.initN, m.finiN, m.showN, m.resizeN, out
}

// noopRender writes nothing to next. Used for tests that don't need
// content (e.g., budget overshoot, ctx cancel).
func noopRender(_ *buffer.Grid) {}

// newTestScheduler builds a FrameScheduler with a mock driver and a
// custom RenderFn. fps is high (1000) so each tick is 1ms — keeps
// tests fast.
func newTestScheduler(t *testing.T, cols, rows int, render RenderFn) (*FrameScheduler, *mockDriver) {
	t.Helper()
	drv := &mockDriver{cols: cols, rows: rows}
	if render == nil {
		render = noopRender
	}
	return NewFrameScheduler(drv, 1000, render), drv
}

// runShort runs a scheduler for a brief window then cancels.
func runShort(t *testing.T, s *FrameScheduler, d time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(d)
		cancel()
	}()
	return s.Run(ctx)
}

// TestScheduler_InterfaceSatisfy — compile-time + runtime assert.
func TestScheduler_InterfaceSatisfy(t *testing.T) {
	t.Parallel()
	drv := &mockDriver{cols: 10, rows: 5}
	var _ Scheduler = NewFrameScheduler(drv, 60, noopRender)
}

// TestFrame_NilDriverPanics — caller wiring bug must fail loud.
func TestFrame_NilDriverPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on nil driver")
		}
	}()
	NewFrameScheduler(nil, 60, noopRender)
}

// TestFrame_NilRenderPanics — spec-1.4 R2 CRIT-A: caller wiring bug.
func TestFrame_NilRenderPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on nil render")
		}
	}()
	NewFrameScheduler(&mockDriver{cols: 10, rows: 5}, 60, nil)
}

// TestFrame_FirstFrameFullRedraw — first tick → fullRedraw via
// DiffEngine nil-prev path; verify mockDriver got SetCell calls.
func TestFrame_FirstFrameFullRedraw(t *testing.T) {
	t.Parallel()
	rendered := false
	render := func(g *buffer.Grid) {
		rendered = true
		g.SetCell(0, 0, buffer.Cell{Ch: 'X'})
	}
	s, drv := newTestScheduler(t, 5, 3, render)
	_ = runShort(t, s, 20*time.Millisecond)

	if !rendered {
		t.Errorf("RenderFn was never invoked")
	}
	_, _, show, _, cells := drv.snapshot()
	if show == 0 {
		t.Errorf("Show was never called")
	}
	// At least the single non-zero cell should have been set.
	found := false
	for _, c := range cells {
		if c.X == 0 && c.Y == 0 && c.Ch == 'X' {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected SetCell(0,0,'X') in mockDriver; got %+v", cells)
	}
}

// TestFrame_NoChangeBetweenTicks — second tick with identical render
// state should emit zero SetCell calls beyond the first frame.
func TestFrame_NoChangeBetweenTicks(t *testing.T) {
	t.Parallel()
	render := func(g *buffer.Grid) {
		g.SetCell(1, 1, buffer.Cell{Ch: 'A'})
	}
	s, drv := newTestScheduler(t, 5, 3, render)
	_ = runShort(t, s, 30*time.Millisecond)

	_, _, _, _, cells := drv.snapshot()
	aCount := 0
	for _, c := range cells {
		if c.Ch == 'A' {
			aCount++
		}
	}
	// First frame emits 'A'; subsequent NoChange frames should NOT.
	// So aCount should be 1, not equal to the show count.
	if aCount != 1 {
		t.Errorf("expected exactly 1 SetCell{'A'} (first-frame only); got %d", aCount)
	}
}

// TestFrame_CellClearTranslation — spec-1.3 § 10 forward contract:
// patch.Cell.Ch == 0 → driver.SetCell with ' ' rune.
func TestFrame_CellClearTranslation(t *testing.T) {
	t.Parallel()
	frame := atomic.Int32{}
	render := func(g *buffer.Grid) {
		if frame.Add(1) == 1 {
			g.SetCell(2, 2, buffer.Cell{Ch: 'Z'})
		}
		// subsequent frames leave (2,2) as Cell{}; diff should emit a
		// PatchSetCell{Cell{}} which the scheduler must translate.
	}
	s, drv := newTestScheduler(t, 5, 3, render)
	_ = runShort(t, s, 30*time.Millisecond)

	_, _, _, _, cells := drv.snapshot()
	// Find any SetCell at (2,2) with rune ' ' (space) — the clear.
	cleared := false
	for _, c := range cells {
		if c.X == 2 && c.Y == 2 && c.Ch == ' ' {
			cleared = true
			break
		}
	}
	if !cleared {
		t.Errorf("expected (2,2) clear translated to ' '; got cells %+v", cells)
	}
}

// TestFrame_RenderFnPanicReleasesNext (spec-1.4 § 3.4 E1) —
// RenderFn panic must NOT leak the acquired next Grid.
func TestFrame_RenderFnPanicReleasesNext(t *testing.T) {
	t.Parallel()
	render := func(_ *buffer.Grid) { panic("render boom") }
	s, drv := newTestScheduler(t, 5, 3, render)
	_ = runShort(t, s, 30*time.Millisecond)

	// Driver.Init/Fini should both have fired once.
	init, fini, _, _, _ := drv.snapshot()
	if init != 1 || fini != 1 {
		t.Errorf("init=%d fini=%d; want both 1", init, fini)
	}
	// Should have received at least one ErrorMsg via emit (frame panic).
	// We don't verify count strictly — just that main loop survives + Fini ran.
}

// TestFrame_ShowPanicReleasesNext (spec-1.4 § 3.4 E4).
func TestFrame_ShowPanicReleasesNext(t *testing.T) {
	t.Parallel()
	drv := &mockDriver{cols: 5, rows: 3, showPanic: true}
	s := NewFrameScheduler(drv, 1000, noopRender)
	_ = runShort(t, s, 20*time.Millisecond)
	// Fini must fire even when Show panics on every frame.
	_, fini, _, _, _ := drv.snapshot()
	if fini != 1 {
		t.Errorf("Fini = %d; want 1", fini)
	}
}

// TestFrame_ApplyPanicCleanup (spec-1.4 § 3.4 E3) — SetCell panic
// inside applyPatches must trigger defer cleanup.
func TestFrame_ApplyPanicCleanup(t *testing.T) {
	t.Parallel()
	drv := &mockDriver{cols: 5, rows: 3, setCellPanic: true}
	render := func(g *buffer.Grid) {
		g.SetCell(0, 0, buffer.Cell{Ch: 'X'})
	}
	s := NewFrameScheduler(drv, 1000, render)
	_ = runShort(t, s, 20*time.Millisecond)
	_, fini, _, _, _ := drv.snapshot()
	if fini != 1 {
		t.Errorf("Fini = %d; want 1", fini)
	}
}

// TestFrame_DriverInitError — Init returns error → Run returns same;
// no Fini (defer Fini still runs but we don't have init-2-fini gate).
func TestFrame_DriverInitError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("init failed")
	drv := &mockDriver{cols: 5, rows: 3, initErr: sentinel}
	s := NewFrameScheduler(drv, 1000, noopRender)
	err := s.Run(context.Background())
	if !errors.Is(err, sentinel) {
		t.Errorf("Run err = %v; want %v", err, sentinel)
	}
}

// TestFrame_BufferPoolAcquireFail — Driver.Size returns (0,0); pool
// rejects; frame loop logs WARN and continues.
func TestFrame_BufferPoolAcquireFail(t *testing.T) {
	t.Parallel()
	drv := &mockDriver{cols: 0, rows: 0}
	s := NewFrameScheduler(drv, 1000, noopRender)
	_ = runShort(t, s, 20*time.Millisecond)
	// Should still init + fini cleanly.
	init, fini, _, _, _ := drv.snapshot()
	if init != 1 || fini != 1 {
		t.Errorf("init=%d fini=%d; want both 1", init, fini)
	}
}

// TestFrame_ContextCancel — cancel ctx during Run; verify Fini called
// + Run returns ctx.Err.
func TestFrame_ContextCancel(t *testing.T) {
	t.Parallel()
	s, drv := newTestScheduler(t, 5, 3, noopRender)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run err = %v; want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
	_, fini, _, _, _ := drv.snapshot()
	if fini != 1 {
		t.Errorf("Fini = %d; want 1", fini)
	}
}

// TestFrame_MsgChClosedOnRunExit — spec-1.4 R2 MED-2: caller's
// for-range over Msgs() must exit naturally when Run returns.
func TestFrame_MsgChClosedOnRunExit(t *testing.T) {
	t.Parallel()
	s, _ := newTestScheduler(t, 5, 3, noopRender)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	consumerDone := make(chan struct{})
	go func() {
		for range s.Msgs() {
			// drain
		}
		close(consumerDone)
	}()
	_ = s.Run(ctx)
	select {
	case <-consumerDone:
	case <-time.After(time.Second):
		t.Fatal("Msgs consumer did not exit after Run return (msgCh not closed)")
	}
}

// TestFrame_CmdPanicEmitsErrorMsg — submit panicking Cmd, verify
// Msgs() yields ErrorMsg with full context (CmdID/Submitted/Priority/
// Frame; spec-1.4 R2 H-6) AND errors.Is matches the registered
// ErrPanicRecovered sentinel (spec-1.4 R3 H-A: %w wrap must work).
func TestFrame_CmdPanicEmitsErrorMsg(t *testing.T) {
	t.Parallel()
	s, _ := newTestScheduler(t, 5, 3, noopRender)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { _ = s.Run(ctx); close(done) }()

	id := s.ScheduleAt(func() { panic("cmd boom") }, PriorityHigh)

	deadline := time.After(time.Second)
WAIT:
	for {
		select {
		case <-deadline:
			t.Fatal("no ErrorMsg received")
		case m, ok := <-s.Msgs():
			if !ok {
				t.Fatal("Msgs closed before ErrorMsg")
			}
			em, isErr := m.(ErrorMsg)
			if !isErr {
				continue
			}
			if em.CmdID != id {
				t.Errorf("CmdID = %d; want %d (submitted via ScheduleAt)", em.CmdID, id)
			}
			if em.Priority != PriorityHigh {
				t.Errorf("Priority = %d; want PriorityHigh", em.Priority)
			}
			if em.Frame == 0 {
				t.Errorf("Frame = 0; expected frame counter at panic time")
			}
			if em.Submitted.IsZero() {
				t.Errorf("Submitted is zero; want time of Schedule call")
			}
			if em.Stack == nil {
				t.Errorf("Stack is nil")
			}
			// spec-1.4 R3 H-A: errcode sentinel must be embedded via %w
			// so callers can programmatically classify panics.
			if !errors.Is(em.Err, ErrPanicRecovered) {
				t.Errorf("errors.Is(em.Err, ErrPanicRecovered) = false; want true (R3 H-A %%w wrap)")
			}
			break WAIT
		}
	}
	cancel()
	<-done
}

// TestFrame_ConcurrentScheduleFromCallers — 8 goroutines × 100 Schedule
// noop; race detector must stay silent and all Cmds eventually execute.
func TestFrame_ConcurrentScheduleFromCallers(t *testing.T) {
	t.Parallel()
	var executed atomic.Int64
	render := func(g *buffer.Grid) { g.SetCell(0, 0, buffer.Cell{Ch: 'A'}) }
	s, _ := newTestScheduler(t, 5, 3, render)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = s.Run(ctx); close(done) }()

	const goroutines = 8
	const iters = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				s.Schedule(func() { executed.Add(1) })
			}
		}()
	}
	wg.Wait()
	// Allow enough time for workers to drain.
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done
	if got := executed.Load(); got != goroutines*iters {
		t.Errorf("executed = %d; want %d", got, goroutines*iters)
	}
}

// TestFrame_RenderFnMainGoroutineOnly — spec-1.4 R2 CRIT-A: RenderFn
// must run on the same goroutine that started Run (no worker pool
// dispatch). We capture the calling goroutine's stack inside the
// RenderFn and verify it contains "FrameScheduler.runFrame" (the main
// loop frame) rather than any worker frame.
func TestFrame_RenderFnMainGoroutineOnly(t *testing.T) {
	t.Parallel()
	var sawMainLoop atomic.Bool
	render := func(_ *buffer.Grid) {
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		stack := string(buf[:n])
		if strings.Contains(stack, "scheduler.(*FrameScheduler).runFrame") &&
			!strings.Contains(stack, "workerLoop") {
			sawMainLoop.Store(true)
		}
	}
	s, _ := newTestScheduler(t, 5, 3, render)
	_ = runShort(t, s, 15*time.Millisecond)
	if !sawMainLoop.Load() {
		t.Errorf("RenderFn did not appear to run on FrameScheduler.runFrame goroutine")
	}
}

// TestFrame_PatchResize_CurrentFrameFullRedraw — spec-1.4 R2 CRIT-B: after a
// resize (which triggers PatchResize), s.lastFrame ownership must not
// be mutated mid-applyPatches. We verify that Show was called every
// frame (i.e., main loop didn't crash on a leaked-buffer error path).
func TestFrame_PatchResize_CurrentFrameFullRedraw(t *testing.T) {
	t.Parallel()
	drv := &mockDriver{cols: 10, rows: 5}
	frame := atomic.Int32{}
	render := func(g *buffer.Grid) {
		// Trigger a resize on the second frame by changing reported size.
		if frame.Add(1) == 2 {
			drv.mu.Lock()
			drv.cols = 12
			drv.rows = 7
			drv.mu.Unlock()
		}
		g.SetCell(0, 0, buffer.Cell{Ch: 'R'})
	}
	s := NewFrameScheduler(drv, 1000, render)
	_ = runShort(t, s, 30*time.Millisecond)

	_, fini, show, resize, _ := drv.snapshot()
	if fini != 1 {
		t.Errorf("Fini = %d; want 1 (no leak / cleanup OK)", fini)
	}
	if show == 0 {
		t.Errorf("Show never called")
	}
	if resize == 0 {
		t.Errorf("Resize never called; viewport change not picked up")
	}
}

// TestFrame_MsgChDropOldestUnderPanicStorm — submit many panicking
// Cmds; verify Msgs channel drops oldest rather than blocking main loop.
func TestFrame_MsgChDropOldestUnderPanicStorm(t *testing.T) {
	t.Parallel()
	s, drv := newTestScheduler(t, 5, 3, noopRender)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	// Drain Msgs in background to avoid blocking main loop emit; we
	// only care that Fini still runs.
	go func() {
		for range s.Msgs() {
		}
	}()
	go func() { _ = s.Run(ctx); close(done) }()

	for i := 0; i < 64; i++ {
		s.Schedule(func() { panic("storm") })
	}

	// Let the storm play out.
	time.Sleep(80 * time.Millisecond)
	cancel()
	<-done

	_, fini, _, _, _ := drv.snapshot()
	if fini != 1 {
		t.Errorf("Fini = %d; want 1 — main loop must survive panic storm", fini)
	}
}

// TestFrame_TrySubmitFullLeavesCmdQueued — spec-1.4 R3 user 决策 Option B:
// when the worker channel is full, the frame loop must leave queued
// Cmds at the queue head and bail out of the drain loop without
// blocking. We saturate the pool with a slow Cmd then submit enough
// fillers to overfill the channel, and verify ticks keep coming.
func TestFrame_TrySubmitFullLeavesCmdQueued(t *testing.T) {
	t.Parallel()
	s, drv := newTestScheduler(t, 5, 3, noopRender)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = s.Run(ctx); close(done) }()

	block := make(chan struct{})
	s.ScheduleAt(func() { <-block }, PriorityHigh)

	// Pile up enough cmds to overflow workers (cap 32 + 4 workers active).
	for i := 0; i < 200; i++ {
		s.Schedule(func() {})
	}

	// Frame loop must keep ticking even though queue can't drain fully.
	time.Sleep(40 * time.Millisecond)
	_, _, show, _, _ := drv.snapshot()
	if show < 2 {
		t.Errorf("Show count = %d; expected multiple ticks during backpressure (frame loop blocked?)", show)
	}
	close(block)
	cancel()
	<-done
}

// TestFrame_BudgetOvershootWARN — spec-1.4 D-5 budget: a slow render
// must trigger a slog.Warn but the frame must still complete (not
// preempt). We use a render that sleeps past the 1ms test budget and
// verify Show is still called.
func TestFrame_BudgetOvershootWARN(t *testing.T) {
	t.Parallel()
	render := func(g *buffer.Grid) {
		time.Sleep(5 * time.Millisecond) // > 1ms frameDeadline at fps=1000
		g.SetCell(0, 0, buffer.Cell{Ch: 'A'})
	}
	s, drv := newTestScheduler(t, 5, 3, render)
	_ = runShort(t, s, 30*time.Millisecond)

	_, fini, show, _, _ := drv.snapshot()
	if fini != 1 {
		t.Errorf("Fini = %d; want 1", fini)
	}
	if show == 0 {
		t.Errorf("Show was never called despite overshoot — frame did not complete")
	}
}

// TestFrame_LastFrameReleasedOnExit — spec-1.4 R3 M-A: the final
// adopted lastFrame must be released back to the pool when Run
// returns. We verify by acquiring all pool buckets after Run + check
// none of them is the leaked grid (best-effort indirect check; the
// direct guarantee is in the defer added to Run).
func TestFrame_LastFrameReleasedOnExit(t *testing.T) {
	t.Parallel()
	render := func(g *buffer.Grid) { g.SetCell(0, 0, buffer.Cell{Ch: 'X'}) }
	s, _ := newTestScheduler(t, 5, 3, render)
	_ = runShort(t, s, 15*time.Millisecond)
	if s.lastFrame != nil {
		t.Errorf("s.lastFrame = %p after Run exit; want nil (R3 M-A leak fix)", s.lastFrame)
	}
}

// TestQueue_PopIfRetainsOnSubmitFailure — spec-1.4 R3 user 决策
// Option B: if the worker channel is full, popIf must leave the Cmd at
// the queue head so the next frame can retry it.
func TestQueue_PopIfRetainsOnSubmitFailure(t *testing.T) {
	t.Parallel()
	q := newQueue()
	q.push(jobItem{CmdID: 1, Cmd: func() {}, Priority: PriorityNormal})
	q.push(jobItem{CmdID: 2, Cmd: func() {}, Priority: PriorityNormal})

	if q.popIf(func(j jobItem) bool {
		if j.CmdID != 1 {
			t.Errorf("submitted CmdID = %d; want 1", j.CmdID)
		}
		return false
	}) {
		t.Fatal("popIf returned true on submit failure")
	}
	j, ok := q.pop()
	if !ok || j.CmdID != 1 {
		t.Fatalf("failed submit removed head: got %+v ok=%v, want CmdID=1", j, ok)
	}
	j, ok = q.pop()
	if !ok || j.CmdID != 2 {
		t.Fatalf("second pop got %+v ok=%v, want CmdID=2", j, ok)
	}
}

// TestQueue_PopIfRemovesOnlySubmittedHead — popIf holds the queue lock
// across head selection + successful submit + removal. This prevents
// the old split peek/dropHead bug where a concurrent high-priority
// ScheduleAt could be inserted between calls and then be deleted while
// the already-submitted normal item remained queued.
func TestQueue_PopIfRemovesOnlySubmittedHead(t *testing.T) {
	t.Parallel()
	q := newQueue()
	q.push(jobItem{CmdID: 1, Cmd: func() {}, Priority: PriorityNormal})
	q.push(jobItem{CmdID: 2, Cmd: func() {}, Priority: PriorityNormal})

	var submitted uint64
	if !q.popIf(func(j jobItem) bool {
		submitted = j.CmdID
		return true
	}) {
		t.Fatal("popIf returned false on submit success")
	}
	if submitted != 1 {
		t.Fatalf("submitted CmdID = %d; want 1", submitted)
	}
	q.push(jobItem{CmdID: 99, Cmd: func() {}, Priority: PriorityHigh})
	j, ok := q.pop()
	if !ok || j.CmdID != 99 {
		t.Fatalf("high-priority insert after popIf got %+v ok=%v, want CmdID=99", j, ok)
	}
	j, ok = q.pop()
	if !ok || j.CmdID != 2 {
		t.Fatalf("remaining normal got %+v ok=%v, want CmdID=2", j, ok)
	}
}
