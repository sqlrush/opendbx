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

const okJSON = `{"strict":true,"contexts":["validate","build-linux","unit-test"]}`

const okCI = `name: CI
on: { push: { branches: [main] } }
jobs:
  validate:
    name: validate
    runs-on: ubuntu-latest
    steps: [{ run: "true" }]
  build-linux:
    name: build-linux
    runs-on: ubuntu-latest
    steps: [{ run: "true" }]
  unit-test:
    name: unit-test
    runs-on: ubuntu-latest
    steps: [{ run: "true" }]
  legacy-validate:
    name: Validate (lint / fmt / vet)
    runs-on: ubuntu-latest
    steps: [{ run: "true" }]
`

// writeFiles drops the two fixture files in a temp dir and returns paths.
func writeFiles(t *testing.T, ciBody, jsonBody string) (ciPath, jsonPath string) {
	t.Helper()
	dir := t.TempDir()
	ciPath = filepath.Join(dir, "ci.yml")
	jsonPath = filepath.Join(dir, "rcc.json")
	if err := os.WriteFile(ciPath, []byte(ciBody), 0o600); err != nil {
		t.Fatalf("write ci.yml: %v", err)
	}
	if err := os.WriteFile(jsonPath, []byte(jsonBody), 0o600); err != nil {
		t.Fatalf("write json: %v", err)
	}
	return ciPath, jsonPath
}

// --- Path 1: exact 1:1 match -----------------------------------------

func TestRun_ExactMatch(t *testing.T) {
	t.Parallel()
	ciPath, jsonPath := writeFiles(t, okCI, okJSON)
	var out bytes.Buffer
	code := run(ciPath, jsonPath, &out)
	if code != 0 {
		t.Errorf("expected exit 0; got %d; out=%s", code, out.String())
	}
	if !strings.Contains(out.String(), "3 stable jobs match") {
		t.Errorf("expected ok message; got %q", out.String())
	}
	if !strings.Contains(out.String(), "legacy placeholders skipped: 1") {
		t.Errorf("expected legacy count; got %q", out.String())
	}
}

// --- Path 2: missing context (in JSON but not in ci.yml stable jobs) ---

func TestRun_MissingContext(t *testing.T) {
	t.Parallel()
	json := `{"strict":true,"contexts":["validate","build-linux","missing-job"]}`
	ciPath, jsonPath := writeFiles(t, okCI, json)
	var out bytes.Buffer
	code := run(ciPath, jsonPath, &out)
	if code != 1 {
		t.Errorf("expected exit 1; got %d", code)
	}
	if !strings.Contains(out.String(), "missing-job") {
		t.Errorf("expected drift detail; got %q", out.String())
	}
	if !strings.Contains(out.String(), "missing from ci.yml") {
		t.Errorf("expected missing label; got %q", out.String())
	}
}

// --- Path 3: extra stable job (in ci.yml but not in JSON) -------------

func TestRun_ExtraStableJob(t *testing.T) {
	t.Parallel()
	json := `{"strict":true,"contexts":["validate","build-linux"]}`
	ciPath, jsonPath := writeFiles(t, okCI, json)
	var out bytes.Buffer
	code := run(ciPath, jsonPath, &out)
	if code != 1 {
		t.Errorf("expected exit 1; got %d", code)
	}
	if !strings.Contains(out.String(), "unit-test") {
		t.Errorf("expected unit-test in drift; got %q", out.String())
	}
	if !strings.Contains(out.String(), "not in JSON") {
		t.Errorf("expected extra label; got %q", out.String())
	}
}

// --- Path 4: legacy placeholder ignored ------------------------------

func TestRun_LegacyIgnored(t *testing.T) {
	t.Parallel()
	// JSON lists only "validate"; ci.yml has validate (stable) + legacy-validate.
	// Legacy must NOT appear in any drift list.
	ci := `name: CI
on: { push: { branches: [main] } }
jobs:
  validate:
    name: validate
    runs-on: ubuntu-latest
    steps: [{ run: "true" }]
  legacy-validate:
    name: Validate (lint / fmt / vet)
    runs-on: ubuntu-latest
    steps: [{ run: "true" }]`
	jsonBody := `{"strict":true,"contexts":["validate"]}`
	ciPath, jsonPath := writeFiles(t, ci, jsonBody)
	var out bytes.Buffer
	code := run(ciPath, jsonPath, &out)
	if code != 0 {
		t.Errorf("legacy must be skipped; got %d; out=%s", code, out.String())
	}
	if strings.Contains(out.String(), "Validate (lint") {
		t.Errorf("legacy must NOT appear in output; got %q", out.String())
	}
}

// --- Path 5: malformed JSON (parse error) ---------------------------

func TestRun_MalformedJSON(t *testing.T) {
	t.Parallel()
	ciPath, jsonPath := writeFiles(t, okCI, `{not-valid-json`)
	var out bytes.Buffer
	code := run(ciPath, jsonPath, &out)
	if code != 2 {
		t.Errorf("expected exit 2; got %d", code)
	}
}

// --- Path 6: malformed YAML (parse error) ---------------------------

func TestRun_MalformedYAML(t *testing.T) {
	t.Parallel()
	ciPath, jsonPath := writeFiles(t, "{ not: valid: yaml [", okJSON)
	var out bytes.Buffer
	code := run(ciPath, jsonPath, &out)
	if code != 2 {
		t.Errorf("expected exit 2; got %d", code)
	}
}

// --- Path 7: empty contexts array ----------------------------------

func TestRun_EmptyContexts(t *testing.T) {
	t.Parallel()
	json := `{"strict":true,"contexts":[]}`
	ciPath, jsonPath := writeFiles(t, okCI, json)
	var out bytes.Buffer
	code := run(ciPath, jsonPath, &out)
	if code != 2 {
		t.Errorf("expected exit 2 for empty contexts; got %d", code)
	}
	if !strings.Contains(out.String(), "is empty") {
		t.Errorf("expected empty message; got %q", out.String())
	}
}

// --- Path 8: empty context string in array -------------------------

func TestRun_EmptyContextString(t *testing.T) {
	t.Parallel()
	json := `{"strict":true,"contexts":["validate",""]}`
	ciPath, jsonPath := writeFiles(t, okCI, json)
	var out bytes.Buffer
	code := run(ciPath, jsonPath, &out)
	if code != 2 {
		t.Errorf("expected exit 2 for empty string; got %d", code)
	}
}

// --- Path 9: stable job missing 'name:' -----------------------------

func TestRun_JobMissingName(t *testing.T) {
	t.Parallel()
	ci := `name: CI
on: { push: { branches: [main] } }
jobs:
  validate:
    runs-on: ubuntu-latest
    steps: [{ run: "true" }]`
	ciPath, jsonPath := writeFiles(t, ci, `{"strict":true,"contexts":["validate"]}`)
	var out bytes.Buffer
	code := run(ciPath, jsonPath, &out)
	if code != 2 {
		t.Errorf("expected exit 2 for missing name; got %d", code)
	}
	if !strings.Contains(out.String(), "empty 'name:' field") {
		t.Errorf("expected name-field error; got %q", out.String())
	}
}

// --- Path 10: missing files -----------------------------------------

func TestRun_FilesNotFound(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	code := run("/no/such/ci.yml", "/no/such/rcc.json", &out)
	if code != 2 {
		t.Errorf("expected exit 2; got %d", code)
	}
}

// --- Path 11: production fixtures (no crash) ----------------------

func TestRun_ProductionFiles(t *testing.T) {
	t.Parallel()
	ci := "../../.github/workflows/ci.yml"
	json := "../../scripts/ci/branch-protection-required-checks.json"
	if _, err := os.Stat(ci); err != nil {
		t.Skipf("production ci.yml not reachable: %v", err)
	}
	if _, err := os.Stat(json); err != nil {
		t.Skipf("production rcc.json not reachable: %v", err)
	}
	var out bytes.Buffer
	code := run(ci, json, &out)
	// T-7 PR 自身 ship 此工具 + 9 stable jobs; should pass.
	if code != 0 {
		t.Errorf("production ci.yml ↔ rcc.json must be 1:1; got %d; out=%s", code, out.String())
	}
}
