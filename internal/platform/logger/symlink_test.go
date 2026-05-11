// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUpdateLatestLinkCreatesPointer(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	target := filepath.Join(tmp, "session-x.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o600); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	updateLatestLink(target)

	latest := filepath.Join(tmp, "latest")
	switch runtime.GOOS {
	case "windows":
		// Hard link: same inode, same content readable via latest.
		if _, err := os.Stat(latest); err != nil {
			t.Fatalf("latest hard link missing: %v", err)
		}
		got, err := os.ReadFile(latest)
		if err != nil || string(got) != "hello" {
			t.Errorf("latest content mismatch: %q err=%v", got, err)
		}
	default:
		// Symlink: Lstat reveals link type and readlink returns target.
		info, err := os.Lstat(latest)
		if err != nil {
			t.Fatalf("latest symlink missing: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("latest is not a symlink (mode=%v)", info.Mode())
		}
		dest, err := os.Readlink(latest)
		if err != nil {
			t.Fatalf("Readlink: %v", err)
		}
		if dest != target {
			t.Errorf("symlink target = %q, want %q", dest, target)
		}
	}
}

// Atomic re-link: calling updateLatestLink with a new target replaces the
// previous link (CC parity — debug.ts:248-249 unlink-then-symlink).
func TestUpdateLatestLinkRelinks(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	t1 := filepath.Join(tmp, "session-1.txt")
	t2 := filepath.Join(tmp, "session-2.txt")
	for _, p := range []string{t1, t2} {
		if err := os.WriteFile(p, []byte(filepath.Base(p)), 0o600); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}

	updateLatestLink(t1)
	updateLatestLink(t2)

	latest := filepath.Join(tmp, "latest")
	if runtime.GOOS != "windows" {
		dest, err := os.Readlink(latest)
		if err != nil {
			t.Fatalf("Readlink: %v", err)
		}
		if dest != t2 {
			t.Errorf("relink: dest = %q, want %q", dest, t2)
		}
	} else {
		// Hard link → same content as t2.
		got, _ := os.ReadFile(latest)
		if string(got) != "session-2.txt" {
			t.Errorf("hard-link relink content = %q, want session-2.txt", got)
		}
	}
}

// Q4 R3 point #1: failure is best-effort. We force a failure by pointing
// updateLatestLink at a path whose dir contains a file named "latest" that
// we cannot remove (read-only dir scenario simulated). The function must
// not panic.
func TestUpdateLatestLinkBestEffortFailure(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	// Create a directory called "latest" in tmp — that blocks os.Symlink
	// from creating a file at the same path.
	if err := os.Mkdir(filepath.Join(tmp, "latest"), 0o700); err != nil {
		t.Fatalf("seed dir: %v", err)
	}
	target := filepath.Join(tmp, "session-x.txt")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	// Must not panic. Stderr will warn but we don't capture it here — that's
	// tested via the writer-integration test below.
	updateLatestLink(target)
}

// End-to-end: writing a log event triggers lazy-once latest link creation.
func TestLoggerWritesCreatesLatestLink(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	setArgvForTesting(t, "opendbx", "--debug")

	if err := Init(InitInput{SessionID: "lazylink"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Before any write: latest must NOT exist (claude HIGH-3 lazy contract).
	latest := filepath.Join(tmp, ".opendbx", "debug", "latest")
	if _, err := os.Lstat(latest); err == nil {
		t.Fatalf("latest link created eagerly at Init — violates lazy-once contract")
	}

	// Now emit one event.
	L().Info("api: connected", Attr{Key: "event", Value: "api.connect"})
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Latest link must now exist and point to the session log.
	target := filepath.Join(tmp, ".opendbx", "debug", "lazylink.txt")
	if runtime.GOOS != "windows" {
		dest, err := os.Readlink(latest)
		if err != nil {
			t.Fatalf("Readlink latest: %v", err)
		}
		if dest != target {
			t.Errorf("latest → %q, want %q", dest, target)
		}
	}
}

func TestWarnLatestLinkFormat(t *testing.T) {
	// NOT t.Parallel: warnLatestLink reads os.Stderr; TestWarnLatestLinkContainsHint
	// mutates os.Stderr via redirection. Running both in parallel triggers a
	// data race on the os.Stderr global (规则 9 race 0 tolerance).
	warnLatestLink("create", "/tmp/latest", os.ErrPermission, "/tmp/session.txt")
	// No assertion — coverage + non-panic.
}

// Test the warning text contains the Q4 R3 template fields.
func TestWarnLatestLinkContainsHint(t *testing.T) {
	// NOT t.Parallel: see TestWarnLatestLinkFormat — both touch os.Stderr.
	r, w, err := os.Pipe()
	if err != nil {
		t.Skipf("pipe unavailable: %v", err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	prev := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = prev })

	warnLatestLink("create", "/tmp/latest", os.ErrPermission, "/tmp/session-q4.txt")
	_ = w.Close()

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	got := string(buf[:n])
	for _, want := range []string{
		"opendbx: warning:",
		"latest debug log link unavailable",
		"/tmp/latest",
		"--debug-file=<path>",
		"/tmp/session-q4.txt",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("warn message missing %q\n  got: %q", want, got)
		}
	}
}
