// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package uitest

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// echoHelper is invoked as a child process by Term tests. Env switch
// GO_UITEST_HELPER=<mode> selects behavior; otherwise normal test runs.
func TestEchoHelper(t *testing.T) {
	mode := os.Getenv("GO_UITEST_HELPER")
	if mode == "" {
		return
	}
	switch mode {
	case "banner":
		// Print a fixed banner and exit.
		_, _ = os.Stdout.WriteString("> ready\r\n")
		os.Exit(0)
	case "wide":
		_, _ = os.Stdout.WriteString("中文 hello\r\n")
		os.Exit(0)
	case "echo-stdin":
		buf := make([]byte, 64)
		n, _ := os.Stdin.Read(buf)
		_, _ = os.Stdout.Write(buf[:n])
		os.Exit(0)
	case "slow":
		// Take long enough that close() must SIGKILL after timeout.
		time.Sleep(10 * time.Second)
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func helperCmd(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestEchoHelper$")
	cmd.Env = append(os.Environ(), "GO_UITEST_HELPER="+mode)
	return cmd
}

func TestTerm_StartAndBanner(t *testing.T) {
	term := Term(t, helperCmd(t, "banner"), 80, 24)
	term.WaitFor(t, func(*Terminal) bool {
		return strings.Contains(strings.Join(term.CellGrid(), "\n"), "> ready")
	}, time.Second)
	// CellGrid row 0 should contain the banner.
	first := term.CellGrid()[0]
	if !strings.Contains(first, "> ready") {
		t.Errorf("row 0 = %q; want substring '> ready'", first)
	}
}

func TestTerm_VTSizeMatches(t *testing.T) {
	term := Term(t, helperCmd(t, "banner"), 100, 30)
	rows := term.CellGridRunes()
	if len(rows) != 30 {
		t.Errorf("got %d rows, want 30", len(rows))
	}
	if len(rows[0]) != 100 {
		t.Errorf("got %d cols, want 100", len(rows[0]))
	}
}

func TestTerm_Send(t *testing.T) {
	term := Term(t, helperCmd(t, "echo-stdin"), 80, 24)
	// Send a payload then wait for echo back.
	if _, err := term.Send([]byte("hello\r\n")...); err != nil {
		t.Fatalf("Send: %v", err)
	}
	term.WaitFor(t, func(*Terminal) bool {
		return strings.Contains(strings.Join(term.CellGrid(), "\n"), "hello")
	}, time.Second)
}

func TestTerm_CloseIdempotent(t *testing.T) {
	term := Term(t, helperCmd(t, "banner"), 80, 24)
	// Wait for exit, then call close twice. Cleanup also calls close —
	// triple-call must not panic.
	term.WaitFor(t, func(*Terminal) bool {
		return strings.Contains(strings.Join(term.CellGrid(), "\n"), "ready")
	}, time.Second)
	term.close()
	term.close()
	// Cleanup will fire as 3rd close.
}

func TestTerm_CloseKillsSlowChild(t *testing.T) {
	// Use slow helper that doesn't exit naturally — close() must kill.
	term := Term(t, helperCmd(t, "slow"), 80, 24)
	start := time.Now()
	term.close()
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("close() took %v; want < 3s (bounded teardown)", elapsed)
	}
}

func TestTerm_RejectsOutOfRangeSize(t *testing.T) {
	cases := []struct {
		name string
		cols int
		rows int
	}{
		{"cols-zero", 0, 24},
		{"rows-zero", 80, 0},
		{"cols-too-big", 70000, 24},
		{"rows-too-big", 80, 70000},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			mt := &mockT{}
			Term(mt, helperCmd(t, "banner"), c.cols, c.rows)
			if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "uint16 range") {
				t.Errorf("expected fatal 'uint16 range'; got %q", mt.fatalMsg)
			}
		})
	}
}

func TestTerm_StartFails(t *testing.T) {
	// pty.StartWithSize on a non-existent binary should error.
	mt := &mockT{}
	cmd := exec.Command("/definitely/not/a/binary/xyz")
	Term(mt, cmd, 80, 24)
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "pty.StartWithSize") {
		t.Errorf("expected fatal 'pty.StartWithSize'; got %q", mt.fatalMsg)
	}
}
