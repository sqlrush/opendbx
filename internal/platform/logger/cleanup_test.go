// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestGuardPanicEmitsProcessPanicEvent(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	setArgvForTesting(t, "opendbx", "--debug")

	if err := Init(InitInput{SessionID: "panic-e2e"}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// GuardPanic must re-panic after recording + flushing. We catch the
	// re-panic so the test itself does not fail.
	caught := func() (panicked any) {
		defer func() { panicked = recover() }()
		GuardPanic(func() {
			panic("boom from inside")
		})
		return nil
	}()
	if caught == nil {
		t.Fatal("GuardPanic swallowed panic (must re-raise)")
	}
	if got, ok := caught.(string); !ok || got != "boom from inside" {
		t.Errorf("re-panic value = %v, want \"boom from inside\"", caught)
	}

	// Sidecar must contain a process.panic event with the value.
	sidecar := filepath.Join(tmp, ".opendbx", "debug", "panic-e2e.events.jsonl")
	raw, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("sidecar missing: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"event":"process.panic"`) {
		t.Errorf("sidecar missing process.panic event:\n%s", got)
	}
	if !strings.Contains(got, "boom from inside") {
		t.Errorf("sidecar missing panic value:\n%s", got)
	}
	// Stack trace fragment — verify at least the runtime/debug.Stack header
	// or this test file's name appears.
	if !strings.Contains(got, "cleanup_test.go") && !strings.Contains(got, "goroutine") {
		t.Errorf("sidecar missing stack trace:\n%s", got)
	}
}

func TestGuardPanicNoPanicNoOp(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	setArgvForTesting(t, "opendbx", "interact")

	if err := Init(InitInput{SessionID: "no-panic"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	called := false
	GuardPanic(func() { called = true })
	if !called {
		t.Fatal("GuardPanic did not invoke fn")
	}
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestRegisterSignalCleanupIdempotent(t *testing.T) {
	// We can't easily verify the signal handler fires (would race with the
	// test harness). Just verify the sync.Once contract is honoured.
	resetSignalCleanupForTesting(t)
	RegisterSignalCleanup()
	RegisterSignalCleanup()
	RegisterSignalCleanup()
	// No assertion — sync.Once prevents double-register; this test verifies
	// no panic / goroutine leak detectable by -race.
}

func TestCloseBeforeInitReturnsErrNotInitialised(t *testing.T) {
	resetForTesting(t)
	if err := Close(); !errors.Is(err, ErrNotInitialised) {
		t.Errorf("Close before Init = %v, want ErrNotInitialised", err)
	}
}

// resetSignalCleanupForTesting clears the package-level sync.Once so each
// test starts with a fresh handler-registration state. Test-only helper.
func resetSignalCleanupForTesting(t *testing.T) {
	t.Helper()
	signalCleanupOnce = sync.Once{}
}
