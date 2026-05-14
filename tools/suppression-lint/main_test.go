// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeGoFile creates a temp Go file with the given body.
func writeGoFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// --- Path 1: empty tree → OK ----------------------------------------

func TestRun_EmptyTree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out bytes.Buffer
	code := run([]string{dir}, false, &out)
	if code != 0 {
		t.Errorf("expected exit 0; got %d; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "all suppression comments carry spec_ref") {
		t.Errorf("expected OK message; got %q", out.String())
	}
}

// --- Path 2: file with nolint + spec_ref → OK -----------------------

func TestRun_NolintWithSpecRef(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeGoFile(t, dir, "ok.go", `package x

//nolint:gochecknoglobals // spec-0.6 § 2.2.1 canonical Register pattern
var Sentinel = "ok"
`)
	var out bytes.Buffer
	code := run([]string{dir}, false, &out)
	if code != 0 {
		t.Errorf("nolint with spec_ref must pass; got %d", code)
	}
}

// --- Path 3: file with nolint missing spec_ref → FAIL ---------------

func TestRun_NolintMissingSpecRef(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeGoFile(t, dir, "bad.go", `package x

//nolint:gosec // operator-supplied path (no spec_ref!)
var X = "bad"
`)
	var out bytes.Buffer
	code := run([]string{dir}, false, &out)
	if code != 1 {
		t.Errorf("nolint missing spec_ref must fail; got %d; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "[nolint]") {
		t.Errorf("expected nolint family tag; got %q", out.String())
	}
}

// --- Path 4: # nosec G304 with spec_ref → OK ------------------------

func TestRun_NosecWithSpecRef(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeGoFile(t, dir, "ok.go", `package x

import "os"

func Read(p string) { _, _ = os.ReadFile(p) /* #nosec G304 -- spec-0.9 D-2: operator path */ }
`)
	var out bytes.Buffer
	code := run([]string{dir}, false, &out)
	// nosec scanned only when on a // comment, not /* */. Should be OK = 0.
	if code != 0 {
		t.Errorf("non-line-comment nosec ignored; got %d", code)
	}
}

func TestRun_NosecLineCommentWithSpecRef(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeGoFile(t, dir, "ok.go", `package x

import "os"

func Read(p string) { _, _ = os.ReadFile(p) } // #nosec G304 -- spec-0.9 D-2: operator path
`)
	var out bytes.Buffer
	code := run([]string{dir}, false, &out)
	if code != 0 {
		t.Errorf("nosec line comment with spec_ref OK; got %d; out=%s", code, out.String())
	}
}

func TestRun_NosecMissingSpecRef(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeGoFile(t, dir, "bad.go", `package x

import "os"

func Read(p string) { _, _ = os.ReadFile(p) } // #nosec G304
`)
	var out bytes.Buffer
	code := run([]string{dir}, false, &out)
	if code != 1 {
		t.Errorf("nosec missing spec_ref must fail; got %d", code)
	}
	if !strings.Contains(out.String(), "[nosec]") {
		t.Errorf("expected nosec tag; got %q", out.String())
	}
}

// --- Path 5: errcode-lint:exempt with / without spec_ref -----------

func TestRun_ErrcodeLintExemptWithSpecRef(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeGoFile(t, dir, "ok.go", `package x

func Foo() error { return nil } // errcode-lint:exempt -- spec-0.10 D-2: stub
`)
	var out bytes.Buffer
	code := run([]string{dir}, false, &out)
	if code != 0 {
		t.Errorf("errcode-lint:exempt with spec_ref OK; got %d", code)
	}
}

func TestRun_ErrcodeLintExemptMissingSpecRef(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeGoFile(t, dir, "bad.go", `package x

func Foo() error { return nil } // errcode-lint:exempt -- legacy library wrapper
`)
	var out bytes.Buffer
	code := run([]string{dir}, false, &out)
	if code != 1 {
		t.Errorf("errcode-lint:exempt missing spec_ref must fail; got %d", code)
	}
	if !strings.Contains(out.String(), "[errcode-lint]") {
		t.Errorf("expected errcode-lint tag; got %q", out.String())
	}
}

// --- Path 6: spec_ref variants accepted -----------------------------

func TestRun_SpecRefVariants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		{"plain", "//nolint:foo // spec-0.6 reason"},
		{"with-D", "//nolint:foo // spec-0.9 D-2 reason"},
		{"with-section", "//nolint:foo // spec-0.6 § 2.2.1 reason"},
		{"with-letter", "//nolint:foo // spec-0.15a reason"},
		{"with-task", "//nolint:foo // spec-0.8-t10a reason"},
		{"with-3-segment", "//nolint:foo // spec-0.11.5 reason"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeGoFile(t, dir, "ok.go", "package x\n\n"+tc.body+"\nvar X = 1\n")
			var out bytes.Buffer
			code := run([]string{dir}, false, &out)
			if code != 0 {
				t.Errorf("variant %s must pass; got %d; out=%s", tc.name, code, out.String())
			}
		})
	}
}

// --- Path 7: vendor / testdata / _test.go skipped -------------------

func TestRun_SkipPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// All these have nolint WITHOUT spec_ref — must NOT trigger because skipped.
	for _, sub := range []string{"vendor", "testdata"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		writeGoFile(t, filepath.Join(dir, sub), "x.go", "package x\n//nolint:foo // no ref\nvar X = 1\n")
	}
	writeGoFile(t, dir, "main_test.go", "package x\n//nolint:foo // no ref\nvar X = 1\n")

	var out bytes.Buffer
	code := run([]string{dir}, false, &out)
	if code != 0 {
		t.Errorf("skip paths must not trigger; got %d; out=%s", code, out.String())
	}
}

// --- Path 8: mixed - one OK + one fail → exit 1 ---------------------

func TestRun_Mixed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeGoFile(t, dir, "good.go", "package x\n//nolint:foo // spec-0.10 D-2 ok\nvar A = 1\n")
	writeGoFile(t, dir, "bad.go", "package x\n//nolint:foo // missing\nvar B = 2\n")
	var out bytes.Buffer
	code := run([]string{dir}, false, &out)
	if code != 1 {
		t.Errorf("mixed fail must exit 1; got %d", code)
	}
	if strings.Count(out.String(), "[nolint]") != 1 {
		t.Errorf("expected 1 violation; got %q", out.String())
	}
}

// --- Path 9: classify edge cases -----------------------------------

func TestClassify(t *testing.T) {
	t.Parallel()
	// classify's contract (T-13 go-reviewer MED-2): caller must strip the
	// comment marker first via stripCommentMarker. The test feeds raw
	// comment text through the same pipeline scanFile uses.
	cases := []struct {
		in   string
		want Family
	}{
		{"//nolint:foo", FamilyNolint},
		{"// nolint:foo", FamilyNolint},
		{"// #nosec G304", FamilyNosec},
		{"// errcode-lint:exempt -- x", FamilyErrcodeLint},
		{"// govulncheck-exempt:GO-2026-1 -- x", FamilyGovulncheck},
		{"// unrelated comment", Family(-1)},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			body := stripCommentMarker(c.in)
			if got := classify(body); got != c.want {
				t.Errorf("classify(%q) = %v; want %v", c.in, got, c.want)
			}
		})
	}
}

// --- Path 10: production canary (opendbx source) --------------------

func TestRun_ProductionScan(t *testing.T) {
	t.Parallel()
	root := "../../"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("production root not reachable: %v", err)
	}
	var out bytes.Buffer
	code := run([]string{root}, false, &out)
	// T-3.5 retrofit goal: every suppression has spec_ref → code 0.
	// If new suppressions added without spec_ref, this test catches them.
	if code != 0 {
		t.Logf("production scan output:\n%s", out.String())
		t.Errorf("production scan must pass (T-3.5 retrofit goal); got %d", code)
	}
}

// --- Path 11: Family.String() coverage ------------------------------

func TestFamilyString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		f    Family
		want string
	}{
		{FamilyNolint, "nolint"},
		{FamilyNosec, "nosec"},
		{FamilyErrcodeLint, "errcode-lint"},
		{FamilyGovulncheck, "govulncheck-exempt"},
		{Family(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.f.String(); got != c.want {
			t.Errorf("Family(%d).String() = %q; want %q", c.f, got, c.want)
		}
	}
}

// --- Path 12: Violation.String() coverage ---------------------------

func TestViolationString(t *testing.T) {
	t.Parallel()
	v := Violation{
		File:    "foo/bar.go",
		Line:    42,
		Family:  FamilyNolint,
		Comment: "//nolint:gosec // no ref",
	}
	got := v.String()
	if !strings.Contains(got, "[nolint]") || !strings.Contains(got, "foo/bar.go:42") {
		t.Errorf("unexpected: %q", got)
	}
}

// --- Path 13: bad path → exit 2 -------------------------------------

func TestRun_BadPath(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code := run([]string{"/no/such/path-xyz"}, false, &out)
	if code != 2 {
		t.Errorf("bad path must exit 2; got %d", code)
	}
}

// --- Path 14: verbose flag prints summary ---------------------------

func TestRun_Verbose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeGoFile(t, dir, "ok.go", "package x\n//nolint:foo // spec-0.10 D-2 ok\nvar X = 1\n")
	var out bytes.Buffer
	code := run([]string{dir}, true, &out)
	if code != 0 {
		t.Errorf("verbose OK; got %d", code)
	}
	if !strings.Contains(out.String(), "scanned 1 path(s)") {
		t.Errorf("expected verbose summary; got %q", out.String())
	}
}

// --- Path 15: hidden directory (.git) skipped ----------------------

func TestRun_HiddenDirSkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hiddenDir := filepath.Join(dir, ".hidden")
	if err := os.MkdirAll(hiddenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Put a bad file in hidden dir — must NOT trigger.
	writeGoFile(t, hiddenDir, "x.go", "package x\n//nolint:foo // missing\nvar X = 1\n")

	var out bytes.Buffer
	code := run([]string{dir}, false, &out)
	if code != 0 {
		t.Errorf("hidden dir must be skipped; got %d; out=%s", code, out.String())
	}
}

// --- Path 16+: realMain coverage -----------------------------------

func TestRealMain_Default(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeGoFile(t, dir, "ok.go", "package x\n//nolint:foo // spec-0.10 D-2 ok\nvar X = 1\n")
	var out bytes.Buffer
	code := realMain([]string{dir}, &out)
	if code != 0 {
		t.Errorf("realMain default; got %d", code)
	}
}

func TestRealMain_Verbose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeGoFile(t, dir, "ok.go", "package x\n//nolint:foo // spec-0.10 D-2 ok\nvar X = 1\n")
	var out bytes.Buffer
	code := realMain([]string{"-v", dir}, &out)
	if code != 0 {
		t.Errorf("realMain -v; got %d", code)
	}
	if !strings.Contains(out.String(), "scanned") {
		t.Errorf("expected verbose summary; got %q", out.String())
	}
}

func TestRealMain_DefaultPath(t *testing.T) {
	t.Parallel()
	// No paths given → defaults to "." (the test binary CWD).
	// In standard go test environment "." is the package dir; both tools
	// have only safe Go files. Just verify no crash.
	var out bytes.Buffer
	code := realMain([]string{}, &out)
	// Could be 0 (pass) or 1 (violations in package); not 2 (error).
	if code == 2 {
		t.Errorf("realMain default path must not error; got %d", code)
	}
}

func TestRealMain_BadFlag(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code := realMain([]string{"--unknown-flag"}, &out)
	if code != 2 {
		t.Errorf("realMain bad flag must exit 2; got %d", code)
	}
}

// --- non-.go file skipped ---------------------------------

func TestRun_NonGoFileSkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// .yaml file with nolint pattern — must NOT scan.
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("# //nolint:foo // missing\nkey: value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := run([]string{dir}, false, &out)
	if code != 0 {
		t.Errorf("non-go file must be skipped; got %d", code)
	}
}
