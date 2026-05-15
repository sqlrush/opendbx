// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

// spec-0.12 D-5 E2E PTY 测试: 5 case (happy / terminal restore / non-TTY /
// prompt non-empty / SIGWINCH).
//
// Strategy: build opendbx binary into tempdir once; each case execs it.
// PTY cases use creack/pty directly (uitest framework doesn't expose
// cmd.Wait / ProcessState needed for exit-code assertions). Non-PTY
// cases use plain exec.Command with pipe redirects.

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// buildOpendbx compiles cmd/opendbx into the test's TempDir and
// returns the binary path. Built once per test via t.Helper.
func buildOpendbx(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	binPath := filepath.Join(dir, "opendbx")
	cmd := exec.Command("go", "build", "-o", binPath, "github.com/sqlrush/opendbx/cmd/opendbx")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build: %v\nstderr: %s", err, stderr.String())
	}
	return binPath
}

// readForBytes reads from r into a buffer until at least minBytes
// arrive or the context expires. Returns whatever was read so far.
func readForBytes(ctx context.Context, r io.Reader, minBytes int) []byte {
	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	for {
		if ctx.Err() != nil {
			return buf.Bytes()
		}
		// Best-effort non-blocking read via deadline-ish loop.
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
			if buf.Len() >= minBytes {
				return buf.Bytes()
			}
		}
		if err != nil {
			return buf.Bytes()
		}
	}
}

// --- non-TTY E2E ------------------------------------------------------

// TestE2E_NonTTY_ReturnsErrcode: stdin pipe + stdout pipe (no PTY)
// → exit non-zero + stderr contains TERMINAL.NOT_A_TTY.
func TestE2E_NonTTY_ReturnsErrcode(t *testing.T) {
	t.Parallel()
	bin := buildOpendbx(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "interact")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = strings.NewReader("") // explicit empty stdin pipe

	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit; got nil; stderr=%q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "TERMINAL.NOT_A_TTY") {
		t.Errorf("stderr should contain TERMINAL.NOT_A_TTY; got %q", stderr.String())
	}
}

// TestE2E_PromptArg_ReturnsErrcode: opendbx interact hello (non-empty
// prompt) → INTERACT.PROMPT_NOT_IMPLEMENTED errcode.
func TestE2E_PromptArg_ReturnsErrcode(t *testing.T) {
	t.Parallel()
	bin := buildOpendbx(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "interact", "hello")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdin = strings.NewReader("")

	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit; got nil")
	}
	if !strings.Contains(stderr.String(), "INTERACT.PROMPT_NOT_IMPLEMENTED") {
		t.Errorf("stderr should contain INTERACT.PROMPT_NOT_IMPLEMENTED; got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "not yet supported") {
		t.Errorf("stderr should contain errcode message; got %q", stderr.String())
	}
}

// --- PTY E2E ----------------------------------------------------------

// runPTY launches binPath under a PTY with given size. Returns the
// PTY master and the *exec.Cmd. Caller MUST call cleanup() in defer.
func runPTY(t *testing.T, binPath string, cols, rows uint16, args ...string) (*os.File, *exec.Cmd, func()) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	ptyFile, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		t.Fatalf("pty.StartWithSize: %v", err)
	}
	cleanup := func() {
		_ = ptyFile.Close()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	return ptyFile, cmd, cleanup
}

// waitForExit waits up to timeout for cmd.Wait. Returns exit code
// (-1 on timeout / unknown). Drains PTY output concurrently.
func waitForExit(t *testing.T, cmd *exec.Cmd, ptyFile *os.File, timeout time.Duration) (int, []byte) {
	t.Helper()
	doneCh := make(chan error, 1)
	go func() { doneCh <- cmd.Wait() }()

	var out bytes.Buffer
	var outMu sync.Mutex
	stopRead := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			select {
			case <-stopRead:
				return
			default:
			}
			n, err := ptyFile.Read(buf)
			if n > 0 {
				outMu.Lock()
				out.Write(buf[:n])
				outMu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	select {
	case <-doneCh:
		close(stopRead)
		wg.Wait()
		outMu.Lock()
		defer outMu.Unlock()
		return cmd.ProcessState.ExitCode(), out.Bytes()
	case <-time.After(timeout):
		t.Errorf("timeout waiting for cmd.Wait after %s", timeout)
		_ = cmd.Process.Signal(syscall.SIGKILL)
		<-doneCh
		close(stopRead)
		wg.Wait()
		outMu.Lock()
		defer outMu.Unlock()
		return -1, out.Bytes()
	}
}

// TestE2E_PTY_CtrlCExitsZero: PTY happy path → wait 200ms for tcell
// init → send Ctrl+C → exit 0.
func TestE2E_PTY_CtrlCExitsZero(t *testing.T) {
	t.Parallel()
	bin := buildOpendbx(t)
	ptyFile, cmd, cleanup := runPTY(t, bin, 80, 24, "interact")
	defer cleanup()

	// Wait for tcell to init (give it some bytes from PTY).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = readForBytes(ctx, ptyFile, 1)

	// Send Ctrl+C (ETX = 0x03).
	if _, err := ptyFile.Write([]byte{0x03}); err != nil {
		t.Fatalf("write Ctrl+C: %v", err)
	}

	code, _ := waitForExit(t, cmd, ptyFile, 3*time.Second)
	if code != 0 {
		t.Errorf("expected exit code 0; got %d", code)
	}
}

// TestE2E_PTY_SIGWINCH_Survives: send SIGWINCH (via PTY resize) then
// Ctrl+C → process should still exit 0 (resize 不应 crash).
func TestE2E_PTY_SIGWINCH_Survives(t *testing.T) {
	t.Parallel()
	bin := buildOpendbx(t)
	ptyFile, cmd, cleanup := runPTY(t, bin, 80, 24, "interact")
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = readForBytes(ctx, ptyFile, 1)

	// Resize twice.
	if err := pty.Setsize(ptyFile, &pty.Winsize{Cols: 120, Rows: 40}); err != nil {
		t.Logf("Setsize 120x40 failed (non-fatal): %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := pty.Setsize(ptyFile, &pty.Winsize{Cols: 60, Rows: 20}); err != nil {
		t.Logf("Setsize 60x20 failed (non-fatal): %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Ctrl+C to exit.
	if _, err := ptyFile.Write([]byte{0x03}); err != nil {
		t.Fatalf("write Ctrl+C: %v", err)
	}

	code, _ := waitForExit(t, cmd, ptyFile, 3*time.Second)
	if code != 0 {
		t.Errorf("expected exit code 0 after SIGWINCH; got %d", code)
	}
}

// guards against import cycle: errors import is used.
var _ = errors.New
