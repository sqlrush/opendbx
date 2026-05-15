// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package uitest

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestWaitFor_DefaultTimeout(t *testing.T) {
	// Use zero timeout → DefaultWaitTimeout (5s). Banner should arrive
	// well under that, so this is just a non-zero-timeout smoke test.
	term := Term(t, helperCmd(t, "banner"), 80, 24)
	term.WaitFor(t, func(*Terminal) bool {
		return strings.Contains(strings.Join(term.CellGrid(), "\n"), "ready")
	}, 0)
}

func TestWaitFor_Timeout(t *testing.T) {
	// Predicate never returns true → WaitFor must Fatalf.
	term := Term(t, helperCmd(t, "banner"), 80, 24)
	mt := &mockT{}
	start := time.Now()
	term.WaitFor(mt, func(*Terminal) bool { return false }, 100*time.Millisecond)
	if !mt.fatalCalled {
		t.Error("expected fatal on timeout")
	}
	if !strings.Contains(mt.fatalMsg, "timed out") {
		t.Errorf("expected 'timed out' in msg; got %q", mt.fatalMsg)
	}
	if !strings.Contains(mt.fatalMsg, "cell grid:") {
		t.Errorf("expected cell grid dump in msg; got %q", mt.fatalMsg)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("WaitFor returned too soon: %v", elapsed)
	}
}

// --- mockT helper (shared across uitest tests) -----------------------

type mockT struct {
	testing.TB
	fatalCalled bool
	fatalMsg    string
}

func (m *mockT) Helper() {}

func (m *mockT) Fatalf(format string, args ...any) {
	m.fatalCalled = true
	m.fatalMsg = fmt.Sprintf(format, args...)
}

func (m *mockT) Cleanup(_ func()) {}
