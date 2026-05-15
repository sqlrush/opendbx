// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package uitest

import (
	"bufio"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

// Terminal wraps a PTY + vt10x parser pair.
//
// Lifecycle:
//   - Term() registers t.Cleanup(close); callers don't manage teardown.
//   - close() is idempotent + race-safe; bounded ≤ 2s total wait.
//   - Two background goroutines: pumpOutput (PTY → vt.Parse) and
//     pumpExit (cmd.Wait → waitCh). Both exit on PTY close.
type Terminal struct {
	cmd      *exec.Cmd
	pty      *os.File
	vt       vt10x.Terminal
	mu       sync.Mutex    // guards closed
	closed   bool          // close() idempotent guard
	waitCh   chan error    // buffered(1); pumpExit sends cmd.Wait err
	pumpDone chan struct{} // closed when pumpOutput returns
}

// Term launches cmd inside a PTY of (cols, rows). The child sees the
// correct WINSIZE on its first byte of output thanks to pty.StartWithSize
// (spec-0.11 R2 codex HIGH-5: pty.Start + Setsize races on first frame).
//
// Calls t.Fatalf on PTY allocation failure or out-of-range size.
func Term(t testing.TB, cmd *exec.Cmd, cols, rows int) *Terminal {
	t.Helper()
	if cols < 1 || cols > 65535 || rows < 1 || rows > 65535 {
		t.Fatalf("uitest.Term: cols/rows out of uint16 range (%d, %d)", cols, rows)
		return nil
	}
	// Mask to 16 bits — gosec G115 has no concern about the conversion
	// because the result is provably in [0, 0xFFFF]. The earlier bounds
	// check above is the semantic invariant; this mask is the compiler
	// hint. spec-0.11 T-6.
	cols16 := uint16(cols & 0xFFFF) //nolint:gosec // spec-0.11 T-6: bounded by mask 0xFFFF
	rows16 := uint16(rows & 0xFFFF) //nolint:gosec // spec-0.11 T-6: bounded by mask 0xFFFF
	ptyFile, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: cols16,
		Rows: rows16,
	})
	if err != nil {
		t.Fatalf("uitest: pty.StartWithSize: %v", err)
		return nil
	}
	vt := vt10x.New(vt10x.WithSize(cols, rows))
	term := &Terminal{
		cmd:      cmd,
		pty:      ptyFile,
		vt:       vt,
		waitCh:   make(chan error, 1),
		pumpDone: make(chan struct{}),
	}
	go term.pumpOutput()
	go term.pumpExit()
	t.Cleanup(term.close)
	return term
}

// pumpOutput reads PTY bytes into the vt10x parser until EOF.
// On EOF, closes pumpDone so close() can synchronize.
func (term *Terminal) pumpOutput() {
	defer close(term.pumpDone)
	_ = term.vt.Parse(bufio.NewReader(term.pty))
}

// pumpExit blocks on cmd.Wait and forwards its error to waitCh.
func (term *Terminal) pumpExit() {
	term.waitCh <- term.cmd.Wait()
}

// close tears down the PTY and reaps the child. Idempotent; safe to
// call from multiple test cleanups. Bounded total wait ≤ 2s:
//
//	500ms — pumpOutput EOF after pty.Close
//	1s    — pumpExit returns from cmd.Wait
//	500ms — SIGKILL drain (if Wait didn't return)
//
// Never calls t.Fatal; Cleanup must not block test teardown reporting.
func (term *Terminal) close() {
	term.mu.Lock()
	if term.closed {
		term.mu.Unlock()
		return
	}
	term.closed = true
	term.mu.Unlock()

	_ = term.pty.Close()

	select {
	case <-term.pumpDone:
	case <-time.After(500 * time.Millisecond):
	}

	select {
	case <-term.waitCh:
		return
	case <-time.After(time.Second):
	}

	_ = term.cmd.Process.Kill()
	select {
	case <-term.waitCh:
	case <-time.After(500 * time.Millisecond):
	}
}
