// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package version

import (
	"runtime"
	"strings"
	"testing"
)

// withMetadata temporarily overrides the package-level metadata vars for
// the duration of f, restoring originals via t.Cleanup. Tests that exercise
// Verbose() under different build states use this — NOT t.Parallel because
// the vars are package globals (claude code-review symlink_test race
// lesson: don't t.Parallel global mutations).
func withMetadata(t *testing.T, version, commit, buildDate, dirty string, f func()) {
	t.Helper()
	origVersion, origCommit, origBuild, origDirty := Version, Commit, BuildDate, Dirty
	t.Cleanup(func() {
		Version, Commit, BuildDate, Dirty = origVersion, origCommit, origBuild, origDirty
	})
	Version, Commit, BuildDate, Dirty = version, commit, buildDate, dirty
	f()
}

// --- Defaults (unset) --------------------------------------------------

func TestDefaults(t *testing.T) {
	// Snapshot what `go test` sees with no -X injection. Cannot t.Parallel
	// because other tests in this file mutate the same globals.
	if Version == "" {
		t.Error("Version default must not be empty")
	}
	// CI may run with ldflags injected, so only assert the defaults case
	// when Version is the literal "dev" sentinel (mirrors the documented
	// default in version.go).
	if Version == "dev" {
		if Commit != "unknown" {
			t.Errorf("Commit default = %q, want unknown when Version=dev", Commit)
		}
		if BuildDate != "unknown" {
			t.Errorf("BuildDate default = %q, want unknown when Version=dev", BuildDate)
		}
		if Dirty != "" {
			t.Errorf("Dirty default = %q, want empty when Version=dev", Dirty)
		}
	}
}

// --- String() ----------------------------------------------------------

func TestStringReturnsVersion(t *testing.T) {
	withMetadata(t, "v0.7.0-stage0.7", "abc123def456", "2026-05-11T10:00:00Z", "", func() {
		if got := String(); got != "v0.7.0-stage0.7" {
			t.Errorf("String() = %q, want v0.7.0-stage0.7", got)
		}
	})
}

// --- Verbose() 6 fixture states ----------------------------------------

func TestVerbose_ReleaseClean(t *testing.T) {
	withMetadata(t, "v0.7.0-stage0.7", "abc123def456", "2026-05-11T10:00:00Z", "", func() {
		got := Verbose()
		assertContains(t, got,
			"opendbx v0.7.0-stage0.7",
			"commit:     abc123def456",
			"built:      2026-05-11T10:00:00Z",
			"workdir:    clean",
			"go:         "+runtime.Version(),
			"os/arch:    "+runtime.GOOS+"/"+runtime.GOARCH,
		)
		// MED-3 防回归: Version 不应含 -dirty 后缀
		if strings.Contains(Version, "-dirty") {
			t.Errorf("Version should never contain -dirty suffix (Dirty var carries that): %q", Version)
		}
	})
}

func TestVerbose_ReleaseDirty(t *testing.T) {
	withMetadata(t, "v0.7.0-stage0.7", "abc123def456", "2026-05-11T10:00:00Z", "dirty", func() {
		got := Verbose()
		assertContains(t, got,
			"opendbx v0.7.0-stage0.7",
			"workdir:    dirty",
		)
		// Critical: Version field still clean — dirty is its own line.
		if strings.Contains(Version, "-dirty") {
			t.Errorf("Version should never contain -dirty (Dirty var owns dirtiness): %q", Version)
		}
		if !strings.Contains(got, "opendbx v0.7.0-stage0.7\n") {
			t.Errorf("Version line should be exact tag without dirty suffix: %q", got)
		}
	})
}

func TestVerbose_DevClean(t *testing.T) {
	withMetadata(t, "dev", "unknown", "unknown", "", func() {
		got := Verbose()
		assertContains(t, got,
			"opendbx dev",
			"commit:     unknown",
			"built:      unknown",
			"workdir:    clean",
		)
	})
}

func TestVerbose_DevDirty(t *testing.T) {
	withMetadata(t, "dev", "unknown", "unknown", "dirty", func() {
		got := Verbose()
		assertContains(t, got,
			"opendbx dev",
			"workdir:    dirty",
		)
	})
}

func TestVerbose_Unset(t *testing.T) {
	// All defaults (empty Dirty included). Equivalent to a fresh go install
	// without any -X overrides — should be a working diagnostic, not panic.
	withMetadata(t, "dev", "unknown", "unknown", "", func() {
		got := Verbose()
		if got == "" {
			t.Fatal("Verbose() returned empty string")
		}
		assertContains(t, got,
			"workdir:    clean",
			"go:         "+runtime.Version(),
		)
	})
}

func TestVerbose_DetachedHEAD(t *testing.T) {
	// `git describe --tags --always` in a detached HEAD past the last tag
	// produces something like "v0.7.0-stage0.7-3-gabc123" — the parser
	// would reject this, but Verbose() must still emit it verbatim
	// (diagnostic priority).
	withMetadata(t, "v0.7.0-stage0.7-3-gabc123", "abc123def456", "2026-05-11T10:00:00Z", "", func() {
		got := Verbose()
		if !strings.Contains(got, "opendbx v0.7.0-stage0.7-3-gabc123") {
			t.Errorf("Verbose should include raw Version even for non-canonical strings: %q", got)
		}
	})
}

// --- Layout structural checks -----------------------------------------

func TestVerbose_LineCount(t *testing.T) {
	withMetadata(t, "v0.7.0-stage0.7", "abc123def456", "2026-05-11T10:00:00Z", "", func() {
		got := Verbose()
		// Trailing \n means 6 content lines + 1 empty after final \n.
		lines := strings.Split(got, "\n")
		if len(lines) != 7 {
			t.Errorf("Verbose() should emit 6 lines + trailing newline, got %d lines: %q", len(lines), got)
		}
	})
}

func TestVerbose_EndsWithNewline(t *testing.T) {
	withMetadata(t, "v0.7.0-stage0.7", "abc", "now", "", func() {
		if !strings.HasSuffix(Verbose(), "\n") {
			t.Error("Verbose() should end with newline")
		}
	})
}

// --- helpers -----------------------------------------------------------

func assertContains(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			t.Errorf("Verbose() output missing %q\n--- got:\n%s", n, haystack)
		}
	}
}
