// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package main implements ci-protection-check, a drift detector between
// .github/workflows/ci.yml stable-name job 'name:' fields and
// scripts/ci/branch-protection-required-checks.json 'contexts' list.
// spec-0.9 D-5 / T-7 (R2 codex HIGH-6 修).
//
// Why this exists:
//
// spec-0.9 D-5's core invariant is: "9 required contexts in branch
// protection match ci.yml job names exactly 1:1". A misaligned context
// silently disables that gate — a stale name in protection means "this
// gate is never reported by CI", so the gate is effectively dead.
//
// This tool reads both files and fails on:
//   - context in JSON that has no matching ci.yml job 'name:' (stale gate)
//   - stable-name ci.yml job that is not in JSON (missing gate)
//
// Legacy placeholder jobs (T-5 兼容 PR 模式; whose 'name:' is a quoted
// historical context like "Validate (lint / fmt / vet)") are SKIPPED —
// they are deleted in T-9 and not gating in the final state.
//
// We identify legacy placeholders by their job key prefix `legacy-`.
//
// Usage:
//
//	go run ./tools/ci-protection-check                                    # use defaults
//	go run ./tools/ci-protection-check -ci=path/to/ci.yml -json=path/to/x # custom paths
//
// Exit codes:
//
//	0  ci.yml stable-name set == JSON contexts set (exact)
//	1  drift detected (missing, extra, or renamed)
//	2  parse failure or unreadable file
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// requiredChecksFile mirrors scripts/ci/branch-protection-required-checks.json.
type requiredChecksFile struct {
	Strict   bool     `json:"strict"`
	Contexts []string `json:"contexts"`
}

// ciYAML mirrors the minimal subset of .github/workflows/ci.yml we need.
type ciYAML struct {
	Jobs map[string]ciJob `yaml:"jobs"`
}

type ciJob struct {
	Name string `yaml:"name"`
}

// loadJSON reads + validates the required-checks JSON file.
//
// T-7.5 codex MED-1 修: reject duplicate contexts — the 1:1 invariant
// breaks if the JSON list contains the same context twice (only one of
// the GitHub status checks would actually gate; the other reports against
// a phantom).
func loadJSON(path string) ([]string, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- spec-0.9 D-5: operator-supplied config path
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var f requiredChecksFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(f.Contexts) == 0 {
		return nil, fmt.Errorf("%s: .contexts is empty", path)
	}
	seen := make(map[string]struct{}, len(f.Contexts))
	for i, c := range f.Contexts {
		if strings.TrimSpace(c) == "" {
			return nil, fmt.Errorf("%s: context[%d] is empty", path, i)
		}
		if _, dup := seen[c]; dup {
			return nil, fmt.Errorf("%s: duplicate context %q at index %d", path, c, i)
		}
		seen[c] = struct{}{}
	}
	return f.Contexts, nil
}

// loadCIJobNames parses ci.yml and returns:
//   - stableNames: jobs whose key does NOT start with "legacy-"
//   - legacyNames: jobs whose key starts with "legacy-" (informational)
//
// The 'name:' field of each job (not the job key) is what GitHub uses as
// the status-check context, so we extract that.
//
// T-7.5 codex MED-1 修: reject duplicate stable-job 'name:' values — if
// two jobs render the same GitHub context string, only one of them can
// gate the PR, making the other a phantom.
func loadCIJobNames(path string) (stableNames, legacyNames []string, err error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- spec-0.9 D-5: operator-supplied config path
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc ciYAML
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(doc.Jobs) == 0 {
		return nil, nil, fmt.Errorf("%s: no jobs found", path)
	}
	stableSeen := make(map[string]string)
	for key, job := range doc.Jobs {
		name := strings.TrimSpace(job.Name)
		if name == "" {
			return nil, nil, fmt.Errorf("%s: job %q has empty 'name:' field", path, key)
		}
		if strings.HasPrefix(key, "legacy-") {
			legacyNames = append(legacyNames, name)
			continue
		}
		if prev, dup := stableSeen[name]; dup {
			return nil, nil, fmt.Errorf("%s: duplicate stable job name %q (jobs %q and %q)",
				path, name, prev, key)
		}
		stableSeen[name] = key
		stableNames = append(stableNames, name)
	}
	sort.Strings(stableNames)
	sort.Strings(legacyNames)
	return stableNames, legacyNames, nil
}

// diff computes set-difference (a - b) returning items in a not in b.
func diff(a, b []string) []string {
	bset := make(map[string]struct{}, len(b))
	for _, v := range b {
		bset[v] = struct{}{}
	}
	out := make([]string, 0)
	for _, v := range a {
		if _, ok := bset[v]; !ok {
			out = append(out, v)
		}
	}
	return out
}

// check compares stable ci.yml job names with JSON contexts.
// Returns "" if exact match, else a human-readable drift message.
func check(stable, contexts []string) string {
	missing := diff(contexts, stable) // in JSON but not in ci.yml stable jobs
	extra := diff(stable, contexts)   // in ci.yml stable jobs but not in JSON
	if len(missing) == 0 && len(extra) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("ci-protection-check FAIL: drift between ci.yml and branch-protection-required-checks.json\n")
	if len(missing) > 0 {
		sb.WriteString(fmt.Sprintf("  contexts in JSON but missing from ci.yml stable jobs (%d):\n", len(missing)))
		for _, m := range missing {
			sb.WriteString("    - " + m + "\n")
		}
	}
	if len(extra) > 0 {
		sb.WriteString(fmt.Sprintf("  ci.yml stable jobs not in JSON contexts (%d):\n", len(extra)))
		for _, e := range extra {
			sb.WriteString("    + " + e + "\n")
		}
	}
	sb.WriteString("  hint: edit scripts/ci/branch-protection-required-checks.json to match ci.yml,\n")
	sb.WriteString("        then run scripts/ci/sync-branch-protection.sh --apply.\n")
	return sb.String()
}

// run is the testable entry point.
func run(ciPath, jsonPath string, w io.Writer) int {
	contexts, err := loadJSON(jsonPath)
	if err != nil {
		_, _ = fmt.Fprintf(w, "ci-protection-check: %v\n", err)
		return 2
	}
	stable, legacy, err := loadCIJobNames(ciPath)
	if err != nil {
		_, _ = fmt.Fprintf(w, "ci-protection-check: %v\n", err)
		return 2
	}
	if msg := check(stable, contexts); msg != "" {
		_, _ = fmt.Fprint(w, msg)
		return 1
	}
	_, _ = fmt.Fprintf(w, "ci-protection-check OK: %d stable jobs match JSON contexts 1:1 (legacy placeholders skipped: %d)\n",
		len(stable), len(legacy))
	return 0
}

func main() {
	ciPath := flag.String("ci", ".github/workflows/ci.yml", "path to ci.yml")
	jsonPath := flag.String("json", "scripts/ci/branch-protection-required-checks.json",
		"path to branch-protection-required-checks.json")
	flag.Parse()
	os.Exit(run(*ciPath, *jsonPath, os.Stderr))
}
