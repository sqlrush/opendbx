// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package golden

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// callerFrameBudget bounds the stack walk in callerDir. 16 frames is
// generous enough for any reasonable testing.T → t.Run → helper chain
// while preventing runaway loops. T-13 go-reviewer LOW-1.
const callerFrameBudget = 16

// Compare diffs got against testdata/golden/<TestName>[/<subname>].golden.
// On -update, writes got to disk (creating intermediate dirs).
// On mismatch without -update, calls t.Errorf with mismatchSummary.
// Binary-safe — no string coercion of payloads.
func Compare(t testing.TB, name string, got []byte) {
	t.Helper()
	path := goldenPath(t, name)
	compareAt(t, path, got)
}

// CompareString is the UTF-8 text convenience overload. For binary or
// non-UTF-8 payloads use Compare with []byte instead.
func CompareString(t testing.TB, name, got string) {
	t.Helper()
	Compare(t, name, []byte(got))
}

// CompareFile compares got against an explicit relative path inside
// the test file's directory. Used for existing corpora where the
// golden file path/suffix is fixed (e.g., cmd/opendbx/testdata/golden/
// version.txt). spec-0.11 R2 codex HIGH-7.
func CompareFile(t testing.TB, relPath string, got []byte) {
	t.Helper()
	path := filepath.Join(callerDir(t), relPath)
	compareAt(t, path, got)
}

// updateOracle is the indirection used by compareAt to read the
// "should we update?" decision. Production binds it to Update(); tests
// can swap via swapUpdateOracleForTest to avoid mutating the real
// -update flag (which races with parallel tests). spec-0.11 T-5.
//
//nolint:gochecknoglobals // spec-0.11 D-3: test seam for race-free flag override.
var updateOracle = Update

func compareAt(t testing.TB, path string, got []byte) {
	t.Helper()
	if updateOracle() {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("golden: mkdir %s: %v", filepath.Dir(path), err)
			return
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("golden: write %s: %v", path, err)
			return
		}
		return
	}
	want, err := os.ReadFile(path) // #nosec G304 -- spec-0.11 D-3: caller-controlled fixture path
	if err != nil {
		t.Fatalf("golden: read %s: %v (run with -update to create)", path, err)
		return
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden mismatch for %s:\n%s", path, mismatchSummary(want, got, 200))
	}
}

// goldenPath returns testdata/golden/<TestName>.golden relative to the
// calling test's file directory. Forward slashes in TestName (from
// t.Run subtests) become subdirectories.
func goldenPath(t testing.TB, name string) string {
	t.Helper()
	testName := t.Name()
	// Subtest paths like "Parent/Sub" → "Parent/Sub.golden".
	rel := filepath.Join("testdata", "golden", testName)
	if name != "" {
		rel = filepath.Join(rel, name)
	}
	return filepath.Join(callerDir(t), rel+".golden")
}

// callerDir returns the directory of the user's test file. We walk
// the call stack skipping non-test frames inside this package; the
// first frame outside this package's non-test code is the test file
// (which may itself be inside this package for self-tests).
func callerDir(t testing.TB) string {
	t.Helper()
	for skip := 2; skip < callerFrameBudget; skip++ {
		_, file, _, ok := runtime.Caller(skip)
		if !ok {
			break
		}
		// Skip our own non-test frames; also skip uitest helper frames
		// so SnapshotGolden goldens land under the caller's test dir
		// (T-13 codex HIGH-2). Tests (_test.go) inside our own package
		// remain valid callers for self-tests of CompareFile.
		if strings.Contains(file, "/internal/testing/golden/") && !strings.HasSuffix(file, "_test.go") {
			continue
		}
		if strings.Contains(file, "/internal/testing/uitest/") && !strings.HasSuffix(file, "_test.go") {
			continue
		}
		return filepath.Dir(file)
	}
	// Last-resort fallback: use cwd. This may give wrong paths if test
	// is run from outside its package dir, but at least won't panic.
	dir, _ := os.Getwd()
	return dir
}

// mismatchSummary renders the first byte/line of divergence between
// want and got plus a truncated tail. NOT a real unified diff — there
// are no context hunks. Output capped at maxLines.
//
// spec-0.11 R2 codex LOW-2: name reflects what it actually is.
func mismatchSummary(want, got []byte, maxLines int) string {
	if bytes.Equal(want, got) {
		return "(no diff)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "want %d bytes, got %d bytes\n", len(want), len(got))

	// Find first divergent byte.
	min := len(want)
	if len(got) < min {
		min = len(got)
	}
	firstDiff := min
	for i := 0; i < min; i++ {
		if want[i] != got[i] {
			firstDiff = i
			break
		}
	}
	wantLine := lineAt(want, firstDiff)
	gotLine := lineAt(got, firstDiff)
	fmt.Fprintf(&b, "first difference at byte %d (line %d)\n",
		firstDiff, lineNumber(want, firstDiff))
	fmt.Fprintf(&b, "  want: %s\n", truncate(wantLine, 200))
	fmt.Fprintf(&b, "  got:  %s\n", truncate(gotLine, 200))

	// Show byte tail context (last 200 bytes of each side).
	wantTail, gotTail := tail(want, 200), tail(got, 200)
	if !bytes.Equal(wantTail, gotTail) {
		fmt.Fprintf(&b, "tail want: %q\n", wantTail)
		fmt.Fprintf(&b, "tail got:  %q\n", gotTail)
	}
	_ = maxLines // reserved for future hunk capping
	return b.String()
}

// lineAt extracts the full text line containing byte index pos.
func lineAt(b []byte, pos int) string {
	if pos >= len(b) {
		return ""
	}
	start := pos
	for start > 0 && b[start-1] != '\n' {
		start--
	}
	end := pos
	for end < len(b) && b[end] != '\n' {
		end++
	}
	return string(b[start:end])
}

// lineNumber returns the 1-based line number of byte index pos.
func lineNumber(b []byte, pos int) int {
	n := 1
	for i := 0; i < pos && i < len(b); i++ {
		if b[i] == '\n' {
			n++
		}
	}
	return n
}

// truncate caps s to maxLen with "..." suffix if too long.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// tail returns the last n bytes of b, or all of b if shorter.
func tail(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[len(b)-n:]
}
