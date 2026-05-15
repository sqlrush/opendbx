// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package tui

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

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
	before := runtime.NumGoroutine()
	s := initSim(t)
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
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("Run did not return within 500ms after cancel")
	}
	if !errors.Is(gotErr, context.Canceled) {
		t.Errorf("Run after cancel: want context.Canceled, got %v", gotErr)
	}
	// goroutine no-leak gate: allow brief grace for runtime scheduler.
	time.Sleep(50 * time.Millisecond)
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
	// Force tcell to fail Init via $TERM. The real NewScreen path goes
	// through tcell.NewScreen() which loads terminfo from $TERM.
	t.Setenv("TERM", "definitely-not-a-real-terminal-name-xyz")
	_, err := NewScreen()
	if err == nil {
		t.Fatal("expected NewScreen to fail with bogus TERM; got nil err")
	}
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
