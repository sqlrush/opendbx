// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package main

import (
	"bytes"
	"sort"
	"strings"
	"testing"
)

const fixturesBufferPath = "example.com/paint-pattern-lint-fixtures/buffer"

// ============================================================
// End-to-end Lint runs against fixture packages
// ============================================================

// TestLint_BadPkg_ReportsAllPatterns covers PAINT-1 detection across the
// three positive scenarios:
//   - direct: dst.SetCell(x, y, src.Cell(...))
//   - one-hop: c := src.Cell(...); dst.SetCell(x, y, c)
//   - one-hop with non-exiting guard: IsContinuation check without
//     continue/break/return MUST still report.
func TestLint_BadPkg_ReportsAllPatterns(t *testing.T) {
	t.Parallel()
	vs, err := Lint("testdata/fixtures", []string{"./badpkg"}, fixturesBufferPath)
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	got := map[string]int{}
	for _, v := range vs {
		got[v.Function+":"+v.Kind]++
	}
	want := map[string]int{
		"BadDirectInline:direct":            1,
		"BadOneHopUnguarded:one-hop":        1,
		"BadOneHopGuardDoesNotExit:one-hop": 1,
	}
	if len(got) != len(want) {
		t.Errorf("violation count mismatch: got=%v want=%v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("missing or wrong-count violation %s: got=%d want=%d (full set %v)", k, got[k], v, got)
		}
	}
}

// TestLint_GoodPkg_ZeroViolations covers the negative paths:
//   - same-receiver one-hop / direct
//   - guarded one-hop with continue/return exit
//   - exempt directive
//   - Cell{} literal (no taint source)
func TestLint_GoodPkg_ZeroViolations(t *testing.T) {
	t.Parallel()
	vs, err := Lint("testdata/fixtures", []string{"./goodpkg"}, fixturesBufferPath)
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if len(vs) != 0 {
		var sb strings.Builder
		for _, v := range vs {
			sb.WriteString("  " + v.String() + "\n")
		}
		t.Errorf("goodpkg should have zero violations; got %d:\n%s", len(vs), sb.String())
	}
}

// TestLint_TestFile_SkippedByDefault verifies a _test.go file with an
// intentional violation is NOT reported absent the include directive.
func TestLint_TestFile_SkippedByDefault(t *testing.T) {
	t.Parallel()
	vs, err := Lint("testdata/fixtures", []string{"./testfile"}, fixturesBufferPath)
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if len(vs) != 0 {
		t.Errorf("test file should be skipped by default; got %d violations", len(vs))
	}
}

// TestLint_IncludedTestFile_Reported verifies the
// paint-pattern-lint:include opt-in flips a _test.go file into scan.
func TestLint_IncludedTestFile_Reported(t *testing.T) {
	t.Parallel()
	vs, err := Lint("testdata/fixtures", []string{"./includedfile"}, fixturesBufferPath)
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if len(vs) != 1 {
		t.Errorf("included test file should report 1 violation; got %d", len(vs))
	}
}

// ============================================================
// realMain exit code + output
// ============================================================

func TestRealMain_BadPkg_ExitCode1(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code := realMain([]string{"-dir", "testdata/fixtures", "-bufferpkg", fixturesBufferPath, "./badpkg"}, &out)
	if code != 1 {
		t.Errorf("exit code = %d; want 1 (violations present)", code)
	}
	if !strings.Contains(out.String(), "FAIL") || !strings.Contains(out.String(), "PAINT-1") {
		t.Errorf("output should mention FAIL + PAINT-1; got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "paint.Blit / BlitAt") {
		t.Errorf("hint message missing helper reference; got:\n%s", out.String())
	}
}

func TestRealMain_GoodPkg_ExitCode0(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code := realMain([]string{"-dir", "testdata/fixtures", "-bufferpkg", fixturesBufferPath, "./goodpkg"}, &out)
	if code != 0 {
		t.Errorf("exit code = %d; want 0; output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "OK") {
		t.Errorf("output should mention OK; got:\n%s", out.String())
	}
}

// ============================================================
// Violation String formatting
// ============================================================

func TestViolation_String_Format(t *testing.T) {
	t.Parallel()
	v := Violation{
		Pkg:      "example.com/x",
		File:     "/tmp/foo.go",
		Line:     42,
		Function: "Bar",
		Kind:     "direct",
		Receiver: "dst",
		Source:   "src",
	}
	got := v.String()
	for _, want := range []string{"[PAINT-1]", "/tmp/foo.go:42", "example.com/x.Bar", "direct pattern", "dst.SetCell", "src.Cell"} {
		if !strings.Contains(got, want) {
			t.Errorf("Violation.String missing %q: %s", want, got)
		}
	}
}

// ============================================================
// exemptPkgs derivation
// ============================================================

func TestExemptPkgs_Derivation(t *testing.T) {
	t.Parallel()
	got := exemptPkgs("github.com/example/render/buffer")
	want := map[string]bool{
		"github.com/example/render/paint":  true,
		"github.com/example/render/buffer": true,
	}
	if len(got) != len(want) {
		t.Errorf("exempt set size = %d; want %d", len(got), len(want))
	}
	for k := range want {
		if !got[k] {
			t.Errorf("exempt set missing %q (got %v)", k, sortedKeys(got))
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
