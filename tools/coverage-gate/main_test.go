// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeProfile creates a temp `go test -coverprofile` file with the given
// content and returns its path.
func writeProfile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "coverage.out")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return path
}

// profileLine fabricates one entry of the raw .out format.
func profileLine(pkg, file string, startLine, numStmts int, covered bool) string {
	count := 0
	if covered {
		count = 1
	}
	return fmt.Sprintf("%s/%s:%d.0,%d.0 %d %d",
		pkg, file, startLine, startLine+1, numStmts, count)
}

// buildProfile assembles a profile with the given lines + mode header.
func buildProfile(lines ...string) string {
	all := append([]string{"mode: set"}, lines...)
	return strings.Join(all, "\n") + "\n"
}

// mustParse parses and fails the test on any error (T-13a go HIGH-1).
func mustParse(t *testing.T, path string) map[string]*PackageCoverage {
	t.Helper()
	pkgs, err := ParseProfile(path)
	if err != nil {
		t.Fatalf("ParseProfile: %v", err)
	}
	return pkgs
}

// --- Path 1: Core ≥ 85% PASS ------------------------------------------

func TestCheck_CorePackagePass(t *testing.T) {
	t.Parallel()
	core := modulePath + "/internal/platform/errcode"
	body := buildProfile(
		profileLine(core, "errcode.go", 10, 9, true),
		profileLine(core, "errcode.go", 20, 1, false),
	)
	pkgs := mustParse(t, writeProfile(t, body))
	violations, _, totalOK := Check(pkgs)
	if len(violations) != 0 {
		t.Errorf("expected no violations; got %v", violations)
	}
	if !totalOK {
		t.Error("total threshold should be met (90%)")
	}
}

// --- Path 2: Core < 85% FAIL ------------------------------------------

func TestCheck_CorePackageFail(t *testing.T) {
	t.Parallel()
	core := modulePath + "/internal/platform/logger"
	body := buildProfile(
		profileLine(core, "logger.go", 10, 8, true),
		profileLine(core, "logger.go", 20, 2, false),
	)
	pkgs := mustParse(t, writeProfile(t, body))
	violations, _, _ := Check(pkgs)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation; got %d: %v", len(violations), violations)
	}
	v := violations[0]
	if v.Tier != TierCore || v.Package != core {
		t.Errorf("wrong violation: %+v", v)
	}
	if v.Percent >= coreThreshold {
		t.Errorf("violation pct %.1f%% should be < %.0f%%", v.Percent, coreThreshold)
	}
}

// --- Path 3: Other ≥ 75% PASS -----------------------------------------

func TestCheck_OtherPackagePass(t *testing.T) {
	t.Parallel()
	other := modulePath + "/internal/platform/profileutil"
	body := buildProfile(
		profileLine(other, "profile.go", 10, 8, true),
		profileLine(other, "profile.go", 20, 2, false),
	)
	pkgs := mustParse(t, writeProfile(t, body))
	violations, _, _ := Check(pkgs)
	if len(violations) != 0 {
		t.Errorf("expected no violations (other tier ≥ 75%%); got %v", violations)
	}
}

// --- Path 4: Other < 75% FAIL -----------------------------------------

func TestCheck_OtherPackageFail(t *testing.T) {
	t.Parallel()
	other := modulePath + "/internal/platform/profileutil"
	body := buildProfile(
		profileLine(other, "profile.go", 10, 7, true),
		profileLine(other, "profile.go", 20, 3, false),
	)
	pkgs := mustParse(t, writeProfile(t, body))
	violations, _, _ := Check(pkgs)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation; got %d", len(violations))
	}
	if violations[0].Tier != TierOther {
		t.Errorf("wrong tier: %v", violations[0].Tier)
	}
}

// --- Path 5: Exempt 包不阻 gate ---------------------------------------

func TestCheck_ExemptPackageNotBlocking(t *testing.T) {
	t.Parallel()
	exempt := modulePath + "/internal/entrypoints"
	body := buildProfile(
		profileLine(exempt, "entrypoints.go", 10, 4, true),
		profileLine(exempt, "entrypoints.go", 20, 6, false),
		profileLine(modulePath+"/internal/platform/errcode", "errcode.go", 10, 9, true),
		profileLine(modulePath+"/internal/platform/errcode", "errcode.go", 20, 1, true),
	)
	pkgs := mustParse(t, writeProfile(t, body))
	violations, totalPct, totalOK := Check(pkgs)
	for _, v := range violations {
		if v.Package == exempt {
			t.Errorf("exempt package %s should not appear in violations: %v", exempt, v)
		}
	}
	if totalPct < 99.9 {
		t.Errorf("exempt leaked into total: got %.1f%%, want 100%%", totalPct)
	}
	if !totalOK {
		t.Error("total should be ≥ 80%")
	}
}

// --- Path 6: Total < 80% FAIL (per-package OK but total falls below) ---

func TestCheck_TotalBelowThresholdFails(t *testing.T) {
	t.Parallel()
	core := modulePath + "/internal/platform/version"
	coreLines := []string{
		profileLine(core, "version.go", 1, 17, true),
		profileLine(core, "version.go", 100, 3, false),
	}
	other := modulePath + "/internal/platform/profileutil"
	otherLines := []string{
		profileLine(other, "profile.go", 1, 750, true),
		profileLine(other, "profile.go", 1000, 250, false),
	}
	body := buildProfile(append(coreLines, otherLines...)...)
	pkgs := mustParse(t, writeProfile(t, body))
	violations, totalPct, totalOK := Check(pkgs)
	if len(violations) != 0 {
		t.Errorf("expected no per-package violations; got %v", violations)
	}
	if totalOK {
		t.Errorf("total %.1f%% should be flagged as below 80%%", totalPct)
	}
	if totalPct >= totalThreshold {
		t.Errorf("expected totalPct < %.0f%%; got %.1f%%", totalThreshold, totalPct)
	}
}

// --- Path 7: Profile parse errors (defensive) -------------------------

func TestParseProfile_MalformedLine(t *testing.T) {
	t.Parallel()
	body := "mode: set\nthis-line-has-no-colon\n"
	_, err := ParseProfile(writeProfile(t, body))
	if err == nil {
		t.Fatal("expected parse error for malformed line")
	}
	if !strings.Contains(err.Error(), "missing ':'") {
		t.Errorf("error should mention missing ':' — got: %v", err)
	}
}

func TestParseProfile_NotInteger(t *testing.T) {
	t.Parallel()
	body := "mode: set\n" + modulePath + "/pkg/file.go:1.0,2.0 NOT_INT 1\n"
	_, err := ParseProfile(writeProfile(t, body))
	if err == nil {
		t.Fatal("expected parse error for non-integer numStmts")
	}
}

func TestParseProfile_MissingSlash(t *testing.T) {
	t.Parallel()
	body := "mode: set\nfile.go:1.0,2.0 1 1\n"
	_, err := ParseProfile(writeProfile(t, body))
	if err == nil {
		t.Fatal("expected parse error for missing '/'")
	}
}

func TestParseProfile_FewerThan3Fields(t *testing.T) {
	t.Parallel()
	body := "mode: set\n" + modulePath + "/pkg/file.go:1.0,2.0 1\n"
	_, err := ParseProfile(writeProfile(t, body))
	if err == nil {
		t.Fatal("expected parse error for fewer than 3 fields")
	}
}

func TestParseProfile_CountNotInt(t *testing.T) {
	t.Parallel()
	body := "mode: set\n" + modulePath + "/pkg/file.go:1.0,2.0 1 ABC\n"
	_, err := ParseProfile(writeProfile(t, body))
	if err == nil {
		t.Fatal("expected parse error for non-integer count")
	}
}

func TestParseProfile_OpenError(t *testing.T) {
	t.Parallel()
	_, err := ParseProfile("/no/such/path-xyz.out")
	if err == nil {
		t.Fatal("expected open error")
	}
}

// --- Path 8: Tier classification ---------------------------------------

func TestClassify(t *testing.T) {
	t.Parallel()
	cases := []struct {
		pkg  string
		want Tier
	}{
		{modulePath + "/internal/platform/errcode", TierCore},
		{modulePath + "/internal/platform/logger", TierCore},
		{modulePath + "/internal/platform/version", TierCore},
		{modulePath + "/internal/entrypoints", TierExempt},
		{modulePath + "/cmd/opendbx", TierExempt},
		{modulePath + "/internal/platform/config", TierExempt},
		{modulePath + "/internal/platform/rpc", TierExempt},
		{modulePath + "/tools/import-rules-check", TierExempt},
		{modulePath + "/tools/coverage-gate", TierTool},
		{modulePath + "/tools/makefile-check", TierTool},
		{modulePath + "/tools/vuln-allowlist", TierTool},
		{modulePath + "/tools/ci-protection-check", TierTool},
		{modulePath + "/tools/suppression-lint", TierTool},
		{modulePath + "/tools/errcode-lint", TierTool},
		{modulePath + "/internal/platform/profileutil", TierOther},
		{modulePath + "/internal/platform/nonexistent", TierOther},
	}
	for _, c := range cases {
		c := c
		t.Run(c.pkg, func(t *testing.T) {
			t.Parallel()
			if got := classify(c.pkg); got != c.want {
				t.Errorf("classify(%q) = %v, want %v", c.pkg, got, c.want)
			}
		})
	}
}

// --- Path 9: Tool tier ≥ 90% threshold (T-13a) ------------------------

func TestCheck_ToolPackageBelowThreshold(t *testing.T) {
	t.Parallel()
	tool := modulePath + "/tools/coverage-gate"
	body := buildProfile(
		profileLine(tool, "main.go", 10, 8, true),
		profileLine(tool, "main.go", 20, 2, false),
	)
	pkgs := mustParse(t, writeProfile(t, body))
	violations, _, _ := Check(pkgs)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation (tool at 80%% < 90%%); got %d", len(violations))
	}
	if violations[0].Tier != TierTool || violations[0].Threshold != toolThreshold {
		t.Errorf("expected TierTool@90; got %+v", violations[0])
	}
}

func TestCheck_ToolPackageAtThreshold(t *testing.T) {
	t.Parallel()
	tool := modulePath + "/tools/makefile-check"
	body := buildProfile(
		profileLine(tool, "main.go", 10, 9, true),
		profileLine(tool, "main.go", 20, 1, false),
	)
	pkgs := mustParse(t, writeProfile(t, body))
	violations, _, _ := Check(pkgs)
	if len(violations) != 0 {
		t.Errorf("expected no violations (tool exactly 90%%); got %v", violations)
	}
}

// --- Path 10: ParseProfile + Percent round-trip ------------------------

func TestParseProfile_PercentMath(t *testing.T) {
	t.Parallel()
	pkg := modulePath + "/x"
	body := buildProfile(
		profileLine(pkg, "a.go", 1, 3, true),
		profileLine(pkg, "a.go", 10, 2, false),
		profileLine(pkg, "b.go", 1, 5, true),
	)
	pkgs := mustParse(t, writeProfile(t, body))
	p, ok := pkgs[pkg]
	if !ok {
		t.Fatalf("package %s not in profile", pkg)
	}
	if p.TotalStmts != 10 {
		t.Errorf("TotalStmts = %d, want 10", p.TotalStmts)
	}
	if p.CoveredStmts != 8 {
		t.Errorf("CoveredStmts = %d, want 8", p.CoveredStmts)
	}
	if got := p.Percent(); got < 79.9 || got > 80.1 {
		t.Errorf("Percent = %.1f, want ~80.0", got)
	}
}

// --- Path 11: InjectMissing — 0% 注入 (T-13a codex MED-3) ------------

func TestInjectMissing_AddsZeroForNonExempt(t *testing.T) {
	t.Parallel()
	pkgs := map[string]*PackageCoverage{
		modulePath + "/x": {Path: modulePath + "/x", TotalStmts: 10, CoveredStmts: 9},
	}
	all := []string{
		modulePath + "/x",                     // already present; skip
		modulePath + "/y",                     // missing other → inject 0
		modulePath + "/internal/entrypoints",  // missing exempt → skip
		modulePath + "/tools/coverage-gate",   // missing tool → inject 0 (violation)
		modulePath + "/internal/platform/rpc", // missing exempt → skip
	}
	InjectMissing(pkgs, all)
	if _, ok := pkgs[modulePath+"/y"]; !ok {
		t.Error("expected /y injected with 0%")
	}
	if _, ok := pkgs[modulePath+"/tools/coverage-gate"]; !ok {
		t.Error("expected tool package injected")
	}
	if _, ok := pkgs[modulePath+"/internal/entrypoints"]; ok {
		t.Error("exempt package should NOT be injected")
	}
	if _, ok := pkgs[modulePath+"/internal/platform/rpc"]; ok {
		t.Error("exempt rpc should NOT be injected")
	}
	if pkgs[modulePath+"/x"].CoveredStmts != 9 {
		t.Error("existing package should not be overwritten")
	}
}

func TestInjectMissing_Empty(t *testing.T) {
	t.Parallel()
	pkgs := map[string]*PackageCoverage{}
	InjectMissing(pkgs, nil)
	if len(pkgs) != 0 {
		t.Errorf("expected empty map; got %d entries", len(pkgs))
	}
}

// --- Path 12: Tier.String + Violation.String --------------------------

func TestTierString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		t    Tier
		want string
	}{
		{TierCore, "core"},
		{TierTool, "tool"},
		{TierOther, "other"},
		{TierExempt, "exempt"},
		{Tier(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.t.String(); got != c.want {
			t.Errorf("Tier(%d).String() = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestViolationString(t *testing.T) {
	t.Parallel()
	v := Violation{Package: "pkg/foo", Tier: TierCore, Percent: 72.3, Threshold: 85.0}
	got := v.String()
	if !strings.Contains(got, "[core]") || !strings.Contains(got, "pkg/foo") ||
		!strings.Contains(got, "72.3%") || !strings.Contains(got, "85%") {
		t.Errorf("unexpected format: %q", got)
	}
}

// --- Path 13: run() — exit paths -------------------------------------

func TestRun_Pass(t *testing.T) {
	t.Parallel()
	core := modulePath + "/internal/platform/errcode"
	body := buildProfile(
		profileLine(core, "errcode.go", 10, 9, true),
		profileLine(core, "errcode.go", 20, 1, true),
	)
	path := writeProfile(t, body)
	var buf bytes.Buffer
	code := run(path, false, &buf, nil)
	if code != 0 {
		t.Errorf("run exit = %d; want 0; out=%s", code, buf.String())
	}
}

func TestRun_VerbosePass(t *testing.T) {
	t.Parallel()
	core := modulePath + "/internal/platform/errcode"
	body := buildProfile(profileLine(core, "errcode.go", 10, 10, true))
	path := writeProfile(t, body)
	var buf bytes.Buffer
	code := run(path, true, &buf, nil)
	if code != 0 {
		t.Errorf("run exit = %d; want 0", code)
	}
	if !strings.Contains(buf.String(), "per-package report") {
		t.Errorf("verbose output missing report header: %s", buf.String())
	}
}

func TestRun_Fail(t *testing.T) {
	t.Parallel()
	core := modulePath + "/internal/platform/errcode"
	body := buildProfile(
		profileLine(core, "errcode.go", 10, 5, true),
		profileLine(core, "errcode.go", 20, 5, false),
	)
	path := writeProfile(t, body)
	var buf bytes.Buffer
	code := run(path, false, &buf, nil)
	if code != 1 {
		t.Errorf("run exit = %d; want 1 (core 50%% < 85%%)", code)
	}
	if !strings.Contains(buf.String(), "coverage-gate FAIL") {
		t.Errorf("expected FAIL output; got %s", buf.String())
	}
}

func TestRun_ParseError(t *testing.T) {
	t.Parallel()
	body := "mode: set\nbad-line-no-colon\n"
	path := writeProfile(t, body)
	var buf bytes.Buffer
	code := run(path, false, &buf, nil)
	if code != 2 {
		t.Errorf("run exit = %d; want 2 (parse error)", code)
	}
}

func TestRun_SkipBypass(t *testing.T) {
	t.Setenv("COVERAGE_GATE_SKIP", "1")
	var buf bytes.Buffer
	code := run("/no/such/file", false, &buf, nil)
	if code != 0 {
		t.Errorf("run exit = %d; want 0 (skip)", code)
	}
	if !strings.Contains(buf.String(), "COVERAGE_GATE_SKIP=1") {
		t.Errorf("expected skip warning: %s", buf.String())
	}
}

func TestRun_WithLister(t *testing.T) {
	t.Parallel()
	core := modulePath + "/internal/platform/errcode"
	body := buildProfile(
		profileLine(core, "errcode.go", 10, 10, true),
	)
	path := writeProfile(t, body)
	fakeLister := func() ([]string, error) {
		// Inject a missing other package → 0% violation.
		return []string{modulePath + "/missing-pkg"}, nil
	}
	var buf bytes.Buffer
	code := run(path, false, &buf, fakeLister)
	if code != 1 {
		t.Errorf("run exit = %d; want 1 (missing pkg = 0%% violation)", code)
	}
}

func TestRun_ListerError(t *testing.T) {
	t.Parallel()
	core := modulePath + "/internal/platform/errcode"
	body := buildProfile(profileLine(core, "errcode.go", 10, 10, true))
	path := writeProfile(t, body)
	errLister := func() ([]string, error) {
		return nil, fmt.Errorf("synthetic lister error")
	}
	var buf bytes.Buffer
	code := run(path, false, &buf, errLister)
	if code != 0 {
		t.Errorf("lister error should not block gate; got exit %d", code)
	}
	if !strings.Contains(buf.String(), "go list failed") {
		t.Errorf("expected lister-error notice: %s", buf.String())
	}
}

// --- Path 14: ListPackages — smoke (skips if `go` unavailable) -------

func TestListPackages_Smoke(t *testing.T) {
	t.Parallel()
	out, err := ListPackages()
	if err != nil {
		t.Skipf("go list unavailable in test env: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected ≥ 1 package from go list ./...")
	}
}
