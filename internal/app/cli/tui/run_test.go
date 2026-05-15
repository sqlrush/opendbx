// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package tui

import (
	"context"
	"errors"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

type fakeTty struct{}

func (fakeTty) Start() error        { return nil }
func (fakeTty) Stop() error         { return nil }
func (fakeTty) Drain() error        { return nil }
func (fakeTty) NotifyResize(func()) {}
func (fakeTty) WindowSize() (tcell.WindowSize, error) {
	return tcell.WindowSize{Width: 80, Height: 24}, nil
}
func (fakeTty) Read([]byte) (int, error)    { return 0, io.EOF }
func (fakeTty) Write(p []byte) (int, error) { return len(p), nil }
func (fakeTty) Close() error                { return nil }

// initSim creates and initializes a SimulationScreen for tests.
func initSim(t *testing.T) tcell.SimulationScreen {
	t.Helper()
	s := NewSimulationScreen()
	if err := s.Init(); err != nil {
		t.Fatalf("SimulationScreen.Init: %v", err)
	}
	t.Cleanup(s.Fini)
	return s
}

// --- exit-key paths ---------------------------------------------------

func TestRun_CtrlCExit(t *testing.T) {
	t.Parallel()
	s := initSim(t)
	go func() {
		// Give Run a chance to enter PollEvent.
		time.Sleep(20 * time.Millisecond)
		s.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)
	}()
	err := Run(context.Background(), s)
	if err != nil {
		t.Errorf("Run with Ctrl+C: want nil, got %v", err)
	}
}

func TestRun_CtrlBackslashExit(t *testing.T) {
	t.Parallel()
	s := initSim(t)
	go func() {
		time.Sleep(20 * time.Millisecond)
		s.InjectKey(tcell.KeyCtrlBackslash, 0, tcell.ModNone)
	}()
	err := Run(context.Background(), s)
	if err != nil {
		t.Errorf("Run with Ctrl+\\: want nil, got %v", err)
	}
}

// --- context cancellation + goroutine no-leak gate -------------------

func TestRun_ContextCancel(t *testing.T) {
	// NOT t.Parallel — uses runtime.NumGoroutine baseline.
	// T-13 L-4: capture baseline AFTER initSim to exclude tcell-internal
	// goroutines (if any) from the leak gate.
	s := initSim(t)
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	var gotErr error
	go func() {
		defer wg.Done()
		gotErr = Run(ctx, s)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	// Run should return within bounded time after cancel.
	// T-13 M-1: 1s timeout (was 500ms) for -race scheduler latency tolerance.
	wg.Wait()
	if !errors.Is(gotErr, context.Canceled) {
		t.Errorf("Run after cancel: want context.Canceled, got %v", gotErr)
	}
	// goroutine no-leak gate: T-13 M-1 / L-4 grace increased to 200ms
	// (was 50ms; -race scheduler slowdown can leave teardown unfinished).
	time.Sleep(200 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before {
		t.Errorf("goroutine leak: before=%d after=%d", before, after)
	}
}

// --- resize -----------------------------------------------------------

func TestRun_Resize(t *testing.T) {
	t.Parallel()
	s := initSim(t)
	go func() {
		time.Sleep(20 * time.Millisecond)
		s.SetSize(120, 40)
		time.Sleep(20 * time.Millisecond)
		s.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)
	}()
	if err := Run(context.Background(), s); err != nil {
		t.Errorf("Run with resize: want nil, got %v", err)
	}
}

func TestRun_SpuriousInterruptIgnored(t *testing.T) {
	t.Parallel()
	s := initSim(t)
	go func() {
		time.Sleep(20 * time.Millisecond)
		if err := s.PostEvent(tcell.NewEventInterrupt(nil)); err != nil {
			t.Errorf("PostEvent interrupt: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
		s.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)
	}()
	if err := Run(context.Background(), s); err != nil {
		t.Errorf("Run with spurious interrupt: want nil, got %v", err)
	}
}

// --- non-exit keys ignored -------------------------------------------

func TestRun_OtherKeysIgnored(t *testing.T) {
	t.Parallel()
	s := initSim(t)
	go func() {
		time.Sleep(20 * time.Millisecond)
		// Non-exit keys: should not return.
		s.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
		s.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
		s.InjectKey(tcell.KeyRune, 'a', tcell.ModNone)
		time.Sleep(30 * time.Millisecond)
		// Now the actual exit key.
		s.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)
	}()
	if err := Run(context.Background(), s); err != nil {
		t.Errorf("Run ignoring non-exit keys: want nil, got %v", err)
	}
}

// --- nil screen -------------------------------------------------------

func TestRun_NilScreen(t *testing.T) {
	t.Parallel()
	if err := Run(context.Background(), nil); !errors.Is(err, ErrInitFailed) {
		t.Errorf("Run(nil): want ErrInitFailed, got %v", err)
	}
}

// --- factory tests ----------------------------------------------------

func TestNewSimulationScreen_Factory(t *testing.T) {
	t.Parallel()
	s := NewSimulationScreen()
	if s == nil {
		t.Fatal("NewSimulationScreen returned nil")
	}
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	s.Fini()
}

func TestNewScreen_TCellInitFailure(t *testing.T) {
	// In the non-TTY unit-test process, NewScreen should fail cleanly while
	// preserving ErrInitFailed wrapping.
	t.Setenv("TERM", "definitely-not-a-real-terminal-name-xyz")
	_, err := NewScreen()
	if err == nil {
		t.Fatal("expected NewScreen to fail with bogus TERM; got nil err")
	}
	if !errors.Is(err, ErrInitFailed) {
		t.Errorf("expected ErrInitFailed wrap; got %v", err)
	}
}

func TestNewScreen_TerminfoFactoryFailure(t *testing.T) {
	// NOT t.Parallel — temporarily swaps package-level constructor seams.
	origTTY := newStdIoTtyFn
	origScreen := newTerminfoScreenFromTtyFn
	newStdIoTtyFn = func() (tcell.Tty, error) {
		return fakeTty{}, nil
	}
	newTerminfoScreenFromTtyFn = func(tcell.Tty) (tcell.Screen, error) {
		return nil, errors.New("terminfo boom")
	}
	t.Cleanup(func() {
		newStdIoTtyFn = origTTY
		newTerminfoScreenFromTtyFn = origScreen
	})

	_, err := NewScreen()
	if !errors.Is(err, ErrInitFailed) {
		t.Errorf("expected ErrInitFailed wrap; got %v", err)
	}
}

// --- errcode ---------------------------------------------------------

func TestErrInitFailed_Errcode(t *testing.T) {
	t.Parallel()
	if ErrInitFailed.Code() != "TERMINAL.INIT_FAILED" {
		t.Errorf("ErrInitFailed.Code() = %q, want TERMINAL.INIT_FAILED", ErrInitFailed.Code())
	}
	var sentinel errcode.Error
	if !errors.As(ErrInitFailed, &sentinel) {
		t.Errorf("ErrInitFailed should satisfy errcode.Error interface")
	}
}
