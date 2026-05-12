// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package main

import (
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

// profileLine fabricates one entry of the raw .out format:
//
//	<pkg>/<file>:<startLine>.0,<startLine+1>.0 <numStmts> <count>
//
// covered=true → count=1; covered=false → count=0.
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

// --- Path 1: Core ≥ 85% PASS ------------------------------------------

func TestCheck_CorePackagePass(t *testing.T) {
	t.Parallel()
	core := modulePath + "/internal/platform/errcode"
	// 10 stmts total, 9 covered → 90% (≥ 85%).
	body := buildProfile(
		profileLine(core, "errcode.go", 10, 9, true),
		profileLine(core, "errcode.go", 20, 1, false),
	)
	path := writeProfile(t, body)
	pkgs, err := ParseProfile(path)
	if err != nil {
		t.Fatalf("ParseProfile: %v", err)
	}
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
	// 10 stmts total, 8 covered → 80% (< 85% core threshold).
	body := buildProfile(
		profileLine(core, "logger.go", 10, 8, true),
		profileLine(core, "logger.go", 20, 2, false),
	)
	path := writeProfile(t, body)
	pkgs, _ := ParseProfile(path)
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
	// 10 stmts, 8 covered → 80% (≥ 75% other; < 85% core but not core).
	body := buildProfile(
		profileLine(other, "profile.go", 10, 8, true),
		profileLine(other, "profile.go", 20, 2, false),
	)
	path := writeProfile(t, body)
	pkgs, _ := ParseProfile(path)
	violations, _, _ := Check(pkgs)
	if len(violations) != 0 {
		t.Errorf("expected no violations (other tier ≥ 75%%); got %v", violations)
	}
}

// --- Path 4: Other < 75% FAIL -----------------------------------------

func TestCheck_OtherPackageFail(t *testing.T) {
	t.Parallel()
	other := modulePath + "/internal/platform/profileutil"
	// 10 stmts, 7 covered → 70% (< 75% other threshold).
	body := buildProfile(
		profileLine(other, "profile.go", 10, 7, true),
		profileLine(other, "profile.go", 20, 3, false),
	)
	path := writeProfile(t, body)
	pkgs, _ := ParseProfile(path)
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
	// 10 stmts, 4 covered → 40% (way under any threshold) — but exempt.
	body := buildProfile(
		profileLine(exempt, "entrypoints.go", 10, 4, true),
		profileLine(exempt, "entrypoints.go", 20, 6, false),
		// Add a core package to give total something non-zero.
		profileLine(modulePath+"/internal/platform/errcode", "errcode.go", 10, 9, true),
		profileLine(modulePath+"/internal/platform/errcode", "errcode.go", 20, 1, true),
	)
	path := writeProfile(t, body)
	pkgs, _ := ParseProfile(path)
	violations, totalPct, totalOK := Check(pkgs)
	// Exempt package must NOT show up in violations.
	for _, v := range violations {
		if v.Package == exempt {
			t.Errorf("exempt package %s should not appear in violations: %v", exempt, v)
		}
	}
	// And it must not count toward total (so total = 10/10 = 100% from errcode alone).
	if totalPct < 99.9 {
		t.Errorf("exempt package leaked into total: got %.1f%%, want 100%% (errcode only)", totalPct)
	}
	if !totalOK {
		t.Error("total should be ≥ 80% (errcode 100% only)")
	}
}

// --- Path 6: Total < 80% FAIL (per-package OK but total falls below) ---

func TestCheck_TotalBelowThresholdFails(t *testing.T) {
	t.Parallel()
	// Construct a scenario where every per-package check passes but the
	// weighted total falls below 80%. Use one tiny core package at 85%
	// and one large "other" at 75% so the weighted average is < 80%.
	core := modulePath + "/internal/platform/version"
	// core: 20 stmts, 17 covered → 85.0% (exactly meets ≥ 85%).
	coreLines := []string{
		profileLine(core, "version.go", 1, 17, true),
		profileLine(core, "version.go", 100, 3, false),
	}
	other := modulePath + "/internal/platform/profileutil"
	// other: 1000 stmts, 750 covered → 75.0% (exactly meets ≥ 75%).
	otherLines := []string{
		profileLine(other, "profile.go", 1, 750, true),
		profileLine(other, "profile.go", 1000, 250, false),
	}
	body := buildProfile(append(coreLines, otherLines...)...)
	path := writeProfile(t, body)
	pkgs, _ := ParseProfile(path)
	violations, totalPct, totalOK := Check(pkgs)
	// Per-package: both exactly at their threshold → no violations.
	if len(violations) != 0 {
		t.Errorf("expected no per-package violations; got %v", violations)
	}
	// Total: (17+750)/(20+1000) = 767/1020 ≈ 75.2% (< 80%).
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
	path := writeProfile(t, body)
	_, err := ParseProfile(path)
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
	path := writeProfile(t, body)
	_, err := ParseProfile(path)
	if err == nil {
		t.Fatal("expected parse error for non-integer numStmts")
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

// --- Path 9: Round-trip ParseProfile + Percent ------------------------

func TestParseProfile_PercentMath(t *testing.T) {
	t.Parallel()
	pkg := modulePath + "/x"
	body := buildProfile(
		profileLine(pkg, "a.go", 1, 3, true),
		profileLine(pkg, "a.go", 10, 2, false),
		profileLine(pkg, "b.go", 1, 5, true),
	)
	path := writeProfile(t, body)
	pkgs, err := ParseProfile(path)
	if err != nil {
		t.Fatalf("ParseProfile: %v", err)
	}
	p, ok := pkgs[pkg]
	if !ok {
		t.Fatalf("package %s not in profile", pkg)
	}
	// Total stmts = 3+2+5 = 10; covered = 3+5 = 8 → 80%.
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
