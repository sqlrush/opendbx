// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package uitest

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

// ErrAnsiBufFull reports that ansiBuf reached maxANSIBufBytes cap and
// dropped subsequent PTY bytes silently (vt10x parsing continued).
// spec-0.11.5 T-4 R5 user MED-4.
var ErrAnsiBufFull = errors.New("uitest: ANSIRaw buffer reached max cap")

// captureWriter is the io.Writer half of io.TeeReader; appends PTY
// bytes to Terminal.ansiBuf under lock with cap enforcement.
type captureWriter struct{ term *Terminal }

func (c *captureWriter) Write(p []byte) (int, error) {
	c.term.ansiBufMu.Lock()
	defer c.term.ansiBufMu.Unlock()
	remaining := maxANSIBufBytes - len(c.term.ansiBuf)
	if remaining <= 0 {
		c.term.ansiBufOverflow = true
		return len(p), nil // tee sink: never propagate error
	}
	if len(p) > remaining {
		c.term.ansiBuf = append(c.term.ansiBuf, p[:remaining]...)
		c.term.ansiBufOverflow = true
		return len(p), nil
	}
	c.term.ansiBuf = append(c.term.ansiBuf, p...)
	return len(p), nil
}

// ANSIRaw returns a defensive copy of all PTY bytes received since
// Term() was called. Safe to call from any goroutine concurrently
// with pumpOutput. Returns ErrAnsiBufFull if buffer cap was reached
// (test should fail with guidance to reduce scope).
//
// spec-0.11.5 T-4 BREAKING patch (spec-0.11 uitest API addition).
// Feeds visualgolden.Render input for Layer 3 pixel diff.
//
// errcode-lint:exempt -- spec-0.11.5 T-4: ErrAnsiBufFull is package-internal sentinel for test harness; full errcode registration deferred to T-13 errata.
func (term *Terminal) ANSIRaw() ([]byte, error) {
	term.ansiBufMu.Lock()
	defer term.ansiBufMu.Unlock()
	out := make([]byte, len(term.ansiBuf))
	copy(out, term.ansiBuf)
	if term.ansiBufOverflow {
		// errcode-lint:exempt -- spec-0.11.5 T-4: ErrAnsiBufFull sentinel deferred to T-13 errata
		return out, ErrAnsiBufFull
	}
	return out, nil
}

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

	// spec-0.11.5 T-4 BREAKING patch: ANSIRaw capture.
	// ansiBuf accumulates PTY bytes for visualgolden.Render input.
	// 10 MiB cap; overflow sets ansiBufOverflow flag (read via ANSIRaw).
	ansiBufMu       sync.Mutex
	ansiBuf         []byte
	ansiBufOverflow bool
}

// maxANSIBufBytes is the ansiBuf cap (10 MiB). spec-0.11.5 R5 user MED-4.
const maxANSIBufBytes = 10 << 20

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
	cols16 := uint16(cols & 0xFFFF) // #nosec G115 -- spec-0.11 T-6: bounded by mask 0xFFFF
	rows16 := uint16(rows & 0xFFFF) // #nosec G115 -- spec-0.11 T-6: bounded by mask 0xFFFF
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
//
// vt10x's Parse drains the reader buffer then returns (NOT until EOF),
// so a single call only handles the first output burst — leaving
// later output from the child unread (e.g., responses to Send).
// Loop until Parse returns an EOF-class error from the underlying
// PTY close. Other errors (read-after-close, EINTR transients) also
// terminate the loop to avoid spinning.
//
// On exit, closes pumpDone so close() can synchronize.
// spec-0.11 T-13 codex HIGH-1 fix.
//
// spec-0.11.5 T-4 BREAKING patch: also captures bytes into ansiBuf
// via io.TeeReader so ANSIRaw() can return them for visualgolden.
func (term *Terminal) pumpOutput() {
	defer close(term.pumpDone)
	tee := io.TeeReader(term.pty, &captureWriter{term: term})
	reader := bufio.NewReader(tee)
	for {
		if err := term.vt.Parse(reader); err != nil {
			return
		}
	}
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
