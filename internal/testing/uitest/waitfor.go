// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package uitest

import (
	"strings"
	"testing"
	"time"
)

// DefaultWaitTimeout is used when WaitFor receives zero timeout.
const DefaultWaitTimeout = 5 * time.Second

// Send writes bytes to the PTY (e.g., user keystrokes). Returns
// (n, err) so callers MAY ignore short writes for keystroke sequences
// but MUST check for sequences exceeding PTY buffer.
func (term *Terminal) Send(bs ...byte) (int, error) {
	return term.pty.Write(bs)
}

// WaitFor blocks until pred returns true or timeout fires. On timeout,
// calls t.Fatalf with a full cell-grid snapshot to aid diagnosis.
//
// Pass time.Duration(0) to use DefaultWaitTimeout.
func (term *Terminal) WaitFor(t testing.TB, pred func(*Terminal) bool, timeout time.Duration) {
	t.Helper()
	if timeout == 0 {
		timeout = DefaultWaitTimeout
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if pred(term) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("uitest.WaitFor: timed out after %s; cell grid:\n%s",
				timeout, strings.Join(term.CellGrid(), "\n"))
			return
		}
		<-ticker.C
	}
}
