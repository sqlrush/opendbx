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

// mustCheck runs Check and fails the test on parse error (T-13b go HIGH-2).
func mustCheck(t *testing.T, path string) []Violation {
	t.Helper()
	violations, err := Check(path)
	if err != nil {
		t.Fatalf("Check(%s): %v", path, err)
	}
	return violations
}

// writeMakefile writes a temp Makefile with the given body. Caller-provided
// "# ..." doc-block is preserved as-is; tests can opt out.
func writeMakefile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "Makefile")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write Makefile: %v", err)
	}
	return path
}

// minimalDocBlock returns a doc block satisfying D-7 binary criterion so
// tests can focus on the rule under test without doc-block noise.
const minimalDocBlock = `# Test fixture Makefile.
#
# Categories:
#   用户日常: build / test
#   CI: lint
#   release: release
#
# Cross-repo: needs ../opendbrb sibling.
#
# Requires GNU make + bash.
`

// --- Path 1: OK fixture (no violations) -------------------------------

func TestCheck_OK(t *testing.T) {
	t.Parallel()
	body := minimalDocBlock + `
.PHONY: build test

build: ## Build the binary
	@echo build

test: ## Run tests
	@echo test
`
	path := writeMakefile(t, body)
	violations, err := Check(path)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(violations) != 0 {
		for _, v := range violations {
			t.Errorf("unexpected violation: %s", v)
		}
	}
}

// --- Path 2: missing-help-comment -------------------------------------

func TestCheck_MissingHelp(t *testing.T) {
	t.Parallel()
	body := minimalDocBlock + `
.PHONY: build test

build:
	@echo no-help

test: ## Run tests
	@echo test
`
	path := writeMakefile(t, body)
	violations := mustCheck(t, path)
	found := false
	for _, v := range violations {
		if v.Kind == VMissingHelp && v.Target == "build" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing-help-comment for `build`; got %+v", violations)
	}
}

// --- Path 3: phony-missing-target -------------------------------------

func TestCheck_PhonyMissing(t *testing.T) {
	t.Parallel()
	body := minimalDocBlock + `
.PHONY: build

build: ## Build the binary
	@echo build

test: ## Run tests
	@echo test
`
	path := writeMakefile(t, body)
	violations := mustCheck(t, path)
	found := false
	for _, v := range violations {
		if v.Kind == VPhonyMissing && v.Target == "test" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected phony-missing-target for `test`; got %+v", violations)
	}
}

// --- Path 4: name-not-kebab-lower -------------------------------------

func TestCheck_NameNotKebab(t *testing.T) {
	t.Parallel()
	body := minimalDocBlock + `
.PHONY: Build_thing

Build_thing: ## Build (bad name)
	@echo build
`
	path := writeMakefile(t, body)
	violations := mustCheck(t, path)
	found := false
	for _, v := range violations {
		if v.Kind == VNameNotKebab && v.Target == "Build_thing" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected name-not-kebab-lower for `Build_thing`; got %+v", violations)
	}
}

// --- Path 5: duplicate-target -----------------------------------------

func TestCheck_DuplicateTarget(t *testing.T) {
	t.Parallel()
	body := minimalDocBlock + `
.PHONY: build

build: ## Build first
	@echo first

build: ## Build second
	@echo second
`
	path := writeMakefile(t, body)
	violations := mustCheck(t, path)
	found := false
	for _, v := range violations {
		if v.Kind == VDuplicateTarget && v.Target == "build" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate-target for `build`; got %+v", violations)
	}
}

// --- Path 6: doc-block-incomplete -------------------------------------

func TestCheck_DocBlockMissingAll(t *testing.T) {
	t.Parallel()
	// Doc block intentionally lacks: category keywords, cross-repo paths,
	// and any shell/make requirement word. Use a generic single-line
	// comment that mentions none of the triggers.
	body := `# Fixture without the three required doc-block elements.

.PHONY: build

build: ## Build
	@echo build
`
	path := writeMakefile(t, body)
	violations := mustCheck(t, path)
	found := false
	for _, v := range violations {
		if v.Kind == VDocBlock {
			found = true
			if !strings.Contains(v.Message, "≥ 3 category") {
				t.Errorf("expected category mention: %s", v.Message)
			}
			if !strings.Contains(v.Message, "cross-repo") {
				t.Errorf("expected cross-repo mention: %s", v.Message)
			}
			if !strings.Contains(v.Message, "GNU make") {
				t.Errorf("expected GNU make mention: %s", v.Message)
			}
		}
	}
	if !found {
		t.Errorf("expected doc-block-incomplete; got %+v", violations)
	}
}

// --- Path 7: .PHONY line continuation rejection (R2 MED-1) ----------

func TestCheck_PhonyContinuationRejected(t *testing.T) {
	t.Parallel()
	body := minimalDocBlock + `
.PHONY: build \
        test

build: ## Build
	@echo build

test: ## Test
	@echo test
`
	path := writeMakefile(t, body)
	violations := mustCheck(t, path)
	found := false
	for _, v := range violations {
		if v.Kind == VPhonyContinue {
			found = true
		}
	}
	if !found {
		t.Errorf("expected phony-line-continuation violation; got %+v", violations)
	}
}

// --- Path 8: multiple .PHONY lines union (allowed) -------------------

func TestCheck_MultiplePhonyLinesUnioned(t *testing.T) {
	t.Parallel()
	body := minimalDocBlock + `
.PHONY: build
.PHONY: test

build: ## Build
	@echo build

test: ## Test
	@echo test
`
	path := writeMakefile(t, body)
	violations := mustCheck(t, path)
	// Filter out doc-block / unrelated violations; assert no phony-missing.
	for _, v := range violations {
		if v.Kind == VPhonyMissing {
			t.Errorf("multiple .PHONY lines should union: %s", v)
		}
	}
}

// --- Path 9: pattern rules + conditionals + includes skipped ---------

func TestCheck_PatternsAndConditionalsSkipped(t *testing.T) {
	t.Parallel()
	body := minimalDocBlock + `
.PHONY: build

ifeq ($(GOOS),linux)
PLATFORM := linux
else
PLATFORM := darwin
endif

include other.mk

%.o: %.c
	@echo pattern rule should be skipped

build: ## Build
	@echo build
`
	path := writeMakefile(t, body)
	violations := mustCheck(t, path)
	// `build` is the only top-level target; should not produce violations
	// related to pattern rules or `ifeq`/`include`.
	for _, v := range violations {
		if v.Target != "" && v.Target != "build" {
			t.Errorf("unexpected violation on non-build target: %s", v)
		}
	}
}

// --- Path 10: production opendbx + opendbrb Makefiles (canary) -------
//
// Skipped if files not reachable from test CWD (tools/makefile-check).
// Production Makefiles are NOT guaranteed to be lint-clean at this point
// — T-10a will fix them; this test simply runs the tool without asserting
// zero violations, ensuring it doesn't crash on real input.

func TestCheck_ProductionFixturesNoCrash(t *testing.T) {
	t.Parallel()
	candidates := []string{
		"../../Makefile",             // opendbx top-level
		"../../../opendbrb/Makefile", // sibling opendbrb top-level
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err != nil {
			t.Logf("skip %s (not present from test CWD): %v", p, err)
			continue
		}
		if _, err := Check(p); err != nil {
			t.Errorf("Check(%s) crashed: %v", p, err)
		}
	}
}

// --- Path 11: help-text-too-long (T-13b codex LOW-1) -----------------

func TestCheck_HelpTooLong(t *testing.T) {
	t.Parallel()
	longHelp := strings.Repeat("x", helpMaxLen+5) // 65 chars
	body := minimalDocBlock + `
.PHONY: build

build: ## ` + longHelp + `
	@echo build
`
	violations := mustCheck(t, writeMakefile(t, body))
	found := false
	for _, v := range violations {
		if v.Kind == VHelpTooLong && v.Target == "build" {
			found = true
			if !strings.Contains(v.Message, "spec § 2.3 #4") {
				t.Errorf("message missing spec reference: %s", v.Message)
			}
		}
	}
	if !found {
		t.Errorf("expected help-text-too-long for `build`; got %+v", violations)
	}
}

func TestCheck_HelpExactlyAtLimit(t *testing.T) {
	t.Parallel()
	help := strings.Repeat("x", helpMaxLen) // exactly 60
	body := minimalDocBlock + `
.PHONY: build

build: ## ` + help + `
	@echo build
`
	violations := mustCheck(t, writeMakefile(t, body))
	for _, v := range violations {
		if v.Kind == VHelpTooLong {
			t.Errorf("60 chars should be at limit (not violation): %s", v.Message)
		}
	}
}

// --- Path 12: Violation.String --------------------------------------

func TestViolationString(t *testing.T) {
	t.Parallel()
	v := Violation{File: "Mk", Line: 5, Kind: VMissingHelp, Target: "build", Message: "missing"}
	got := v.String()
	if !strings.Contains(got, "missing-help-comment") || !strings.Contains(got, "Mk:5") ||
		!strings.Contains(got, `"build"`) {
		t.Errorf("unexpected: %q", got)
	}
	// Doc-block has empty target → omit target= portion.
	v2 := Violation{File: "Mk", Kind: VDocBlock, Message: "incomplete"}
	got2 := v2.String()
	if strings.Contains(got2, "target=") {
		t.Errorf("doc-block should omit target=: %q", got2)
	}
}

// --- Path 13: run() exit paths ---------------------------------------

func TestRun_NoArgs(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	code := run(nil, false, &buf)
	if code != 3 {
		t.Errorf("run nil paths exit = %d; want 3", code)
	}
	if !strings.Contains(buf.String(), "usage:") {
		t.Errorf("expected usage hint: %s", buf.String())
	}
}

func TestRun_OKQuiet(t *testing.T) {
	t.Parallel()
	body := minimalDocBlock + `
.PHONY: build

build: ## Build
	@echo build
`
	path := writeMakefile(t, body)
	var buf bytes.Buffer
	code := run([]string{path}, false, &buf)
	if code != 0 {
		t.Errorf("run OK fixture exit = %d; want 0; out=%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "OK") {
		t.Errorf("expected OK in output: %s", buf.String())
	}
}

func TestRun_Verbose(t *testing.T) {
	t.Parallel()
	body := minimalDocBlock + `
.PHONY: build

build: ## Build
	@echo build
`
	path := writeMakefile(t, body)
	var buf bytes.Buffer
	code := run([]string{path}, true, &buf)
	if code != 0 {
		t.Errorf("verbose OK fixture exit = %d; want 0", code)
	}
	if !strings.Contains(buf.String(), "scanned") {
		t.Errorf("expected scanned in verbose output: %s", buf.String())
	}
}

func TestRun_Fail(t *testing.T) {
	t.Parallel()
	body := minimalDocBlock + `
.PHONY: build

build:
	@echo build
`
	path := writeMakefile(t, body)
	var buf bytes.Buffer
	code := run([]string{path}, false, &buf)
	if code != 1 {
		t.Errorf("run fixture with missing-help exit = %d; want 1", code)
	}
	if !strings.Contains(buf.String(), "FAIL") {
		t.Errorf("expected FAIL output: %s", buf.String())
	}
}

func TestRun_OpenError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	code := run([]string{"/no/such/Makefile-xyz"}, false, &buf)
	if code != 3 {
		t.Errorf("run with missing file exit = %d; want 3", code)
	}
}
