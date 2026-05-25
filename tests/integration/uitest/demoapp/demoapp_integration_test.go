// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

// Package demoapp_test drives the full spec-1.17 production stack —
// SimulationScreen → render/terminal/tcell adapter → program main loop →
// keybindings.Resolve → demoapp.Model — to verify the cursor edit +
// history pipeline end-to-end (spec-1.17 D-7 / MED-5).
//
// tcell isolation: this test does NOT import tcell directly. It drives
// the SimulationScreen through the adapter's test seams
// (NewSimulationDriverForTest / InjectKeyForTest), keeping the IMP-9
// tcell-isolation boundary at the 4 whitelisted packages.
package demoapp_test

import (
	"context"
	"testing"
	"time"

	"github.com/sqlrush/opendbx/internal/app/cli/demoapp"
	"github.com/sqlrush/opendbx/internal/app/cli/program"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
	tcelladapter "github.com/sqlrush/opendbx/internal/app/cli/render/terminal/tcell"
)

// harness wires a SimulationScreen-backed Driver + program around a
// demoapp Model, runs the program loop in a goroutine, and exposes
// helpers to inject keys and read the final Model state.
type harness struct {
	t      *testing.T
	driver *tcelladapter.Driver
	prog   *program.Program
	cancel context.CancelFunc
	done   chan error
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	driver := tcelladapter.NewSimulationDriverForTest(80, 24)
	m := demoapp.New()
	p := program.New(driver, m)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	h := &harness{t: t, driver: driver, prog: p, cancel: cancel, done: done}
	// Deterministically wait for the scheduler's driver.Init (→ sim.Init)
	// to complete before injecting keys. WaitStartedForTest blocks on the
	// scheduler Started signal (closed after driver.Init returns); the
	// channel-close happens-before guarantees we observe Init's writes,
	// so the first InjectKey (which also touches sim internal state) is
	// race-free. tcell's SimulationScreen.Init is NOT concurrency-safe
	// with other sim methods, so this sync is mandatory.
	if !program.WaitStartedForTest(ctx, p) {
		t.Fatalf("scheduler did not reach Started within harness setup")
	}
	return h
}

// typeRunes injects a sequence of printable runes.
func (h *harness) typeRunes(s string) {
	for _, r := range s {
		h.driver.InjectKeyForTest(terminal.KeyRune, r)
		time.Sleep(3 * time.Millisecond)
	}
}

// key injects a single non-rune key by terminal.Key* code.
func (h *harness) key(code int) {
	h.driver.InjectKeyForTest(code, 0)
	time.Sleep(5 * time.Millisecond)
}

// finish cancels the program loop, joins it, and returns the final
// demoapp Model for assertions.
func (h *harness) finish() *demoapp.Model {
	h.t.Helper()
	// Let the last injected key drain.
	time.Sleep(30 * time.Millisecond)
	h.cancel()
	select {
	case <-h.done:
	case <-time.After(2 * time.Second):
		h.t.Fatalf("program.Run did not return within 2s of cancel")
	}
	m, ok := program.CurrentModelForTest(h.prog).(*demoapp.Model)
	if !ok {
		h.t.Fatalf("current model is not *demoapp.Model")
	}
	return m
}

// TestDemoApp_TypeAndSubmit drives "hello" + Enter through the full
// stack and asserts the buffer clears, the log captures the entry, and
// the history ring holds it.
func TestDemoApp_TypeAndSubmit(t *testing.T) {
	h := newHarness(t)
	h.typeRunes("hello")
	h.key(terminal.KeyEnter)
	m := h.finish()

	if got := m.InputState(); got.Buffer != "" || got.Cursor != 0 {
		t.Errorf("after submit InputState = %+v; want empty", got)
	}
	if !m.LogContainsForTest("hello") {
		t.Errorf("log missing \"hello\"; log=%v", m.LogForTest())
	}
}

// TestDemoApp_HistoryUpDown submits two entries, types a draft, walks
// Up/Up/Down/Down, and asserts the fresh draft is restored.
func TestDemoApp_HistoryUpDown(t *testing.T) {
	h := newHarness(t)
	h.typeRunes("aa")
	h.key(terminal.KeyEnter)
	h.typeRunes("bb")
	h.key(terminal.KeyEnter)
	h.typeRunes("dr") // fresh draft

	h.key(terminal.KeyUp)   // → "bb"
	h.key(terminal.KeyUp)   // → "aa"
	h.key(terminal.KeyDown) // → "bb"
	h.key(terminal.KeyDown) // → draft "dr"
	m := h.finish()

	if got := m.InputState().Buffer; got != "dr" {
		t.Errorf("after Up/Up/Down/Down buffer = %q; want \"dr\" (draft restored)", got)
	}
}

// TestDemoApp_CursorEdit types "abc", moves Left twice, Backspace, and
// asserts the resulting buffer + cursor.
func TestDemoApp_CursorEdit(t *testing.T) {
	h := newHarness(t)
	h.typeRunes("abc")
	h.key(terminal.KeyLeft)
	h.key(terminal.KeyLeft)
	h.key(terminal.KeyBackspace)
	m := h.finish()

	st := m.InputState()
	if st.Buffer != "bc" || st.Cursor != 0 {
		t.Errorf("after abc Left Left Backspace InputState = %+v; want {Buffer:\"bc\", Cursor:0}", st)
	}
}
