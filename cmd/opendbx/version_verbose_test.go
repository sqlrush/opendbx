// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// spec-0.7 T-6 — CLI integration tests for --version-verbose.
//
// Output includes runtime.Version() and runtime.GOOS/GOARCH which vary
// across CI matrix [ubuntu, macos]. Pure golden-file equality would be
// brittle, so these tests assert *substring* presence — matching the
// pattern T-4 already established for version.Verbose() unit tests.
//
// "5 fixture state" per spec § 1.1 D-4 maps to 5 metadata permutations
// (release-clean / release-dirty / dev / detached / unset) exercised via
// version package globals mutated before runCmd. NOT t.Parallel because
// the metadata vars are package globals (claude code-reviewer race lesson).

package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/version"
)

// withVersionMetadata snapshots + overrides the version package metadata
// globals for the duration of f, restoring originals via t.Cleanup. Mirrors
// withMetadata in internal/platform/version/version_test.go but accessible
// from cmd/opendbx because the vars are exported.
func withVersionMetadata(t *testing.T, ver, commit, buildDate, dirty string, f func()) {
	t.Helper()
	origV, origC, origB, origD := version.Version, version.Commit, version.BuildDate, version.Dirty
	t.Cleanup(func() {
		version.Version, version.Commit, version.BuildDate, version.Dirty = origV, origC, origB, origD
	})
	version.Version, version.Commit, version.BuildDate, version.Dirty = ver, commit, buildDate, dirty
	f()
}

// --- 5 fixture state via root --version-verbose ----------------------

func TestVersionVerboseFlag_ReleaseClean(t *testing.T) {
	withVersionMetadata(t, "v0.7.0-stage0.7", "abc123def456", "2026-05-11T10:00:00Z", "", func() {
		stdout, _, err := runCmd(t, "--version-verbose")
		if err != nil {
			t.Fatalf("runCmd: %v", err)
		}
		assertCLIContains(t, stdout,
			"opendbx v0.7.0-stage0.7",
			"commit:     abc123def456",
			"built:      2026-05-11T10:00:00Z",
			"workdir:    clean",
			"go:         "+runtime.Version(),
			"os/arch:    "+runtime.GOOS+"/"+runtime.GOARCH,
		)
	})
}

func TestVersionVerboseFlag_ReleaseDirty(t *testing.T) {
	withVersionMetadata(t, "v0.7.0-stage0.7", "abc123def456", "2026-05-11T10:00:00Z", "dirty", func() {
		stdout, _, err := runCmd(t, "--version-verbose")
		if err != nil {
			t.Fatalf("runCmd: %v", err)
		}
		assertCLIContains(t, stdout,
			"opendbx v0.7.0-stage0.7",
			"workdir:    dirty",
		)
		// MED-3 防回归 (CLI-level): Version line must not have -dirty embedded.
		if strings.Contains(stdout, "v0.7.0-stage0.7-dirty") {
			t.Errorf("--version-verbose output leaked -dirty into Version: %q", stdout)
		}
	})
}

func TestVersionVerboseFlag_Dev(t *testing.T) {
	withVersionMetadata(t, "dev", "unknown", "unknown", "", func() {
		stdout, _, err := runCmd(t, "--version-verbose")
		if err != nil {
			t.Fatalf("runCmd: %v", err)
		}
		assertCLIContains(t, stdout,
			"opendbx dev",
			"commit:     unknown",
			"built:      unknown",
			"workdir:    clean",
		)
	})
}

func TestVersionVerboseFlag_DetachedHEAD(t *testing.T) {
	// `git describe` past last tag emits `v0.7.0-stage0.7-3-gabc123` —
	// Verbose() must echo verbatim (diagnostic priority, no Parse).
	withVersionMetadata(t, "v0.7.0-stage0.7-3-gabc123", "abc123def456", "2026-05-11T10:00:00Z", "", func() {
		stdout, _, err := runCmd(t, "--version-verbose")
		if err != nil {
			t.Fatalf("runCmd: %v", err)
		}
		if !strings.Contains(stdout, "opendbx v0.7.0-stage0.7-3-gabc123") {
			t.Errorf("detached-HEAD Version not emitted verbatim: %q", stdout)
		}
	})
}

func TestVersionVerboseFlag_Unset(t *testing.T) {
	// All defaults — should still produce a useful diagnostic block.
	withVersionMetadata(t, "dev", "unknown", "unknown", "", func() {
		stdout, _, err := runCmd(t, "--version-verbose")
		if err != nil {
			t.Fatalf("runCmd: %v", err)
		}
		if stdout == "" {
			t.Fatal("--version-verbose returned empty output")
		}
		// Layout: 6 content lines + trailing newline = 7 split results.
		lines := strings.Split(stdout, "\n")
		if len(lines) != 7 {
			t.Errorf("--version-verbose should emit 6 lines + trailing newline, got %d: %q", len(lines), stdout)
		}
	})
}

// --- Precedence + subcommand integration ----------------------------

func TestVersionVerboseTakesPrecedenceOverVersion(t *testing.T) {
	// spec § 8 Q8 + § 1.1 D-4: --version-verbose wins when both set.
	withVersionMetadata(t, "v0.7.0-stage0.7", "abc", "now", "", func() {
		stdout, _, err := runCmd(t, "--version", "--version-verbose")
		if err != nil {
			t.Fatalf("runCmd: %v", err)
		}
		// Verbose form has multiple lines; single-form has just one line.
		if !strings.Contains(stdout, "commit:     abc") {
			t.Errorf("expected verbose output (multi-line with commit:); got single-line %q", stdout)
		}
		// Ensure single-form fingerprint is NOT what we got.
		// Single form is exactly "<v> (opendbx)\n" — no commit/built/etc.
		if strings.Count(stdout, "\n") < 5 {
			t.Errorf("expected multi-line verbose output; got %d newlines: %q", strings.Count(stdout, "\n"), stdout)
		}
	})
}

func TestVersionSubcommandWithVerboseFlag(t *testing.T) {
	// claude HIGH-4 (c): `opendbx version --version-verbose` must emit the
	// Verbose() block (consistent with the root --version-verbose flag).
	withVersionMetadata(t, "v0.7.0-stage0.7", "abc123def456", "2026-05-11T10:00:00Z", "", func() {
		stdout, _, err := runCmd(t, "version", "--version-verbose")
		if err != nil {
			t.Fatalf("runCmd: %v", err)
		}
		assertCLIContains(t, stdout,
			"opendbx v0.7.0-stage0.7",
			"commit:     abc123def456",
			"workdir:    clean",
		)
	})
}

func TestVersionSubcommandBareUsesSingleForm(t *testing.T) {
	// Regression guard: `opendbx version` (no flag) must still be the
	// single-line CC-parity form.
	withVersionMetadata(t, "v0.7.0-stage0.7", "abc", "now", "", func() {
		stdout, _, err := runCmd(t, "version")
		if err != nil {
			t.Fatalf("runCmd: %v", err)
		}
		if strings.Contains(stdout, "commit:") {
			t.Errorf("bare `opendbx version` should be single-line, got: %q", stdout)
		}
		if !strings.HasPrefix(stdout, "v0.7.0-stage0.7 (opendbx)") {
			t.Errorf("bare `opendbx version` should start with single-form; got %q", stdout)
		}
	})
}

// --- helpers --------------------------------------------------------

func assertCLIContains(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			t.Errorf("output missing %q\n--- got:\n%s", n, haystack)
		}
	}
}
