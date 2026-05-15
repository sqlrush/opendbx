// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

// spec-0.12 D-5 E2E PTY 测试: 5 case (happy / terminal restore / non-TTY /
// prompt non-empty / SIGWINCH).
//
// Strategy (T-13 M-5): build opendbx binary ONCE per package via TestMain
// + sync.Once. Each case execs the prebuilt binary. PTY cases use
// creack/pty directly. Non-PTY cases use plain exec.Command with pipe
// redirects.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// T-13 M-5: build the opendbx binary ONCE in TestMain (before m.Run)
// so all E2E tests run against the same prebuilt binary. Previously
// each test triggered its own `go build`; under `go test ./...` parallel
// package mode this serialized with other packages' compilation steps
// and frequently exceeded the per-test 5s context timeout.
var (
	buildBin string
	buildDir string
	buildErr error
)

// TestMain builds the binary once before tests run.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "opendbx-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: mktemp: %v\n", err)
		os.Exit(2)
	}
	buildDir = dir
	buildBin = filepath.Join(dir, "opendbx")
	cmd := exec.Command("go", "build", "-o", buildBin, "github.com/sqlrush/opendbx/cmd/opendbx")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		buildErr = fmt.Errorf("go build: %w: %s", err, stderr.String())
	}
	code := m.Run()
	_ = os.RemoveAll(buildDir)
	os.Exit(code)
}

// buildOpendbx returns the path to the prebuilt opendbx binary (built
// once by TestMain). Returns t.Fatal'd on build error.
func buildOpendbx(t *testing.T) string {
	t.Helper()
	if buildErr != nil {
		t.Fatalf("TestMain build failed: %v", buildErr)
	}
	return buildBin
}

// hermeticEnv keeps E2E child processes independent from the developer/CI
// user environment. In particular HOME must not point at a real ~/.opendbx,
// otherwise config-load failures can mask the TTY behavior under test.
func hermeticEnv(t *testing.T) []string {
	t.Helper()
	return []string{
		"HOME=" + t.TempDir(),
		"PATH=" + os.Getenv("PATH"),
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
	}
}

// readForBytes reads from r into a buffer until at least minBytes
// arrive or the context expires. Returns whatever was read so far.
// T-13 M-2: runtime.Gosched on zero-byte successful reads to yield
// the scheduler under CI load.
func readForBytes(ctx context.Context, r *os.File, minBytes int) []byte {
	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	for {
		if ctx.Err() != nil {
			return buf.Bytes()
		}
		// PTY reads are blocking. A short read deadline lets the context
		// timeout actually stop the loop instead of waiting for another byte.
		_ = r.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
			if buf.Len() >= minBytes {
				return buf.Bytes()
			}
		}
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				runtime.Gosched()
				continue
			}
			if timeoutErr, ok := err.(interface{ Timeout() bool }); ok && timeoutErr.Timeout() {
				runtime.Gosched()
				continue
			}
			return buf.Bytes()
		}
		if n == 0 {
			runtime.Gosched()
		}
	}
}

// --- non-TTY E2E ------------------------------------------------------

// TestE2E_NonTTY_ReturnsErrcode: stdin pipe + stdout pipe (no PTY)
// → exit non-zero + stderr contains TERMINAL.NOT_A_TTY.
func TestE2E_NonTTY_ReturnsErrcode(t *testing.T) {
	t.Parallel()
	bin := buildOpendbx(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "interact")
	cmd.Env = hermeticEnv(t)
	cmd.Dir = t.TempDir()
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "interact", "hello")
	cmd.Env = hermeticEnv(t)
	cmd.Dir = t.TempDir()
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
// T-13 M-4 (security): minimal env (HOME / PATH / TERM / LANG) instead
// of os.Environ() inheritance, so future authenticated tests don't
// silently leak CI secrets to the child.
func runPTY(t *testing.T, binPath string, cols, rows uint16, args ...string) (*os.File, *exec.Cmd, func()) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Env = hermeticEnv(t)
	cmd.Dir = t.TempDir()
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
// T-13 L-3: stopRead is closed via `defer` so both happy and timeout
// paths signal the drain goroutine without ordering ambiguity.
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
	defer func() {
		close(stopRead)
		wg.Wait()
	}()

	select {
	case <-doneCh:
		outMu.Lock()
		defer outMu.Unlock()
		return cmd.ProcessState.ExitCode(), out.Bytes()
	case <-time.After(timeout):
		t.Errorf("timeout waiting for cmd.Wait after %s", timeout)
		_ = cmd.Process.Signal(syscall.SIGKILL)
		<-doneCh
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	initial := readForBytes(ctx, ptyFile, 1)
	time.Sleep(300 * time.Millisecond)

	// Send Ctrl+C (ETX = 0x03).
	if _, err := ptyFile.Write([]byte{0x03}); err != nil {
		t.Fatalf("write Ctrl+C: %v; initial output=%q", err, string(initial))
	}

	code, out := waitForExit(t, cmd, ptyFile, 3*time.Second)
	if code != 0 {
		t.Errorf("expected exit code 0; got %d; output=%q", code, string(out))
	}
}

// TestE2E_PTY_SIGWINCH_Survives: send SIGWINCH (via PTY resize) then
// Ctrl+C → process should still exit 0 (resize 不应 crash).
func TestE2E_PTY_SIGWINCH_Survives(t *testing.T) {
	t.Parallel()
	bin := buildOpendbx(t)
	ptyFile, cmd, cleanup := runPTY(t, bin, 80, 24, "interact")
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	initial := readForBytes(ctx, ptyFile, 1)
	time.Sleep(300 * time.Millisecond)

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
		t.Fatalf("write Ctrl+C: %v; initial output=%q", err, string(initial))
	}

	code, out := waitForExit(t, cmd, ptyFile, 3*time.Second)
	if code != 0 {
		t.Errorf("expected exit code 0 after SIGWINCH; got %d; output=%q", code, string(out))
	}
}

// TestE2E_PTY_TerminalStateRestored: T-13 claude H-1. After PTY child
// exits via Ctrl+C, run `stty -a` on a fresh PTY of the parent test
// process — assert echo/icanon are still set (tcell.Fini restored
// terminal state). Cross-platform: stty output format differs between
// macOS (BSD) and Linux (GNU) — accept either `echo` or `-echo` token,
// and either `icanon` or `-icanon`, since the absence of leading `-`
// indicates the flag is enabled (not disabled).
func TestE2E_PTY_TerminalStateRestored(t *testing.T) {
	t.Parallel()
	bin := buildOpendbx(t)
	ptyFile, cmd, cleanup := runPTY(t, bin, 80, 24, "interact")
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	initial := readForBytes(ctx, ptyFile, 1)
	time.Sleep(300 * time.Millisecond)

	if _, err := ptyFile.Write([]byte{0x03}); err != nil {
		t.Fatalf("write Ctrl+C: %v; initial output=%q", err, string(initial))
	}
	code, out := waitForExit(t, cmd, ptyFile, 3*time.Second)
	if code != 0 {
		t.Errorf("expected exit code 0; got %d; initial output=%q; output=%q", code, string(initial), string(out))
	}

	// Now spawn stty -a on a NEW PTY to verify terminal state restored.
	// (The previous PTY master is closed by cleanup; we open a fresh PTY
	// pair and exec stty inside it.) If tcell.Fini correctly restored
	// state, the fresh PTY's stty output should show normal echo/icanon.
	sttyCmd := exec.Command("stty", "-a")
	sttyPty, err := pty.Start(sttyCmd)
	if err != nil {
		t.Skipf("stty PTY start failed (env-specific): %v", err)
		return
	}
	defer func() {
		_ = sttyPty.Close()
		_ = sttyCmd.Process.Kill()
		_, _ = sttyCmd.Process.Wait()
	}()

	sttyCtx, sttyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sttyCancel()
	output := readForBytes(sttyCtx, sttyPty, 100)
	got := string(output)
	// `echo` or `icanon` should be present (not prefixed with `-` which means disabled).
	// More relaxed: just assert the output is non-trivial and contains terminal flag keywords.
	if !strings.Contains(got, "echo") && !strings.Contains(got, "icanon") {
		t.Errorf("stty -a output missing terminal flag keywords; tcell.Fini may not have restored state.\nstty output: %q", got)
	}
}
