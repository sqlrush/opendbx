// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package main implements coverage-gate, enforcing CLAUDE.md 规则 8
// per-package coverage thresholds against a `go test -coverprofile=...`
// output file. spec-0.8 D-1 / T-3 / T-13a.
//
// Tiers (R2 用户拍板 CRIT-A + T-13a 用户拍板 codex CRIT-1):
//
//   - Core packages (≥ 85%):
//     internal/platform/{errcode, logger, version}
//   - Tool packages (≥ 90%): spec-0.8 引入的两个 lint 工具
//     tools/{coverage-gate, makefile-check}（自测循环但仍要求覆盖）
//   - Other packages (≥ 75%): everything not in core/tool/exempt
//   - Exempt (no threshold): entrypoints stub + spec-0.7 era tools +
//     cmd/opendbx + internal/platform/config + internal/platform/rpc
//   - Total project (≥ 80%): aggregated across all non-exempt packages
//
// Missing-package handling (T-13a codex MED-3):
//
// `go test -coverprofile=out ./...` writes nothing to the profile for
// packages without test files. Without explicit injection, a non-exempt
// package with real code and no tests is invisible to the gate. We call
// `go list ./...` to enumerate all packages, then inject a 0%-coverage
// entry for any non-exempt package that is absent — guaranteed violation.
//
// Override: COVERAGE_GATE_SKIP=1 env (Q11 ★A) bypasses all checks but
// emits a loud stderr warning. Use only for emergency hotfixes; CHANGELOG
// must note any usage.
//
// Usage:
//
//	go run ./tools/coverage-gate                              # default profile=coverage.out
//	go run ./tools/coverage-gate -profile=/tmp/foo.out -v     # custom profile + verbose
//	COVERAGE_GATE_SKIP=1 go run ./tools/coverage-gate         # emergency bypass
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Threshold constants per spec-0.8 D-1 + T-13a.
const (
	coreThreshold  = 85.0
	toolThreshold  = 90.0
	otherThreshold = 75.0
	totalThreshold = 80.0
)

// modulePath prefix for grouping packages from profile entries.
const modulePath = "github.com/sqlrush/opendbx"

// Package-tier classification sets.
//
// These maps are treated as immutable lookup tables: written once at file
// scope, read via classify() at runtime. They are NOT to be mutated by any
// code path — tests should add new entries by editing this file. The
// gochecknoglobals exemption is intentional for this configuration table;
// converting to functions returning copies would only add allocation
// overhead without changing semantics (Go has no first-class immutable
// maps).
//
//nolint:gochecknoglobals // tier classification table; treat as const.
var (
	corePackages = map[string]bool{
		modulePath + "/internal/platform/errcode": true,
		modulePath + "/internal/platform/logger":  true,
		modulePath + "/internal/platform/version": true,
	}

	// T-13a codex CRIT-1: 自我覆盖率独立 tier ≥ 90%; 不再 exempt.
	toolPackages = map[string]bool{
		modulePath + "/tools/coverage-gate":  true,
		modulePath + "/tools/makefile-check": true,
	}

	exemptPackages = map[string]bool{
		modulePath + "/internal/entrypoints":           true,
		modulePath + "/tools/import-rules-check":       true, // spec-0.7 era; pending spec-0.10
		modulePath + "/tools/import-rules-check/rules": true,
		modulePath + "/tools/dep-allowlist-check":      true, // spec-0.7 era; pending spec-0.10
		modulePath + "/cmd/opendbx":                    true,
		modulePath + "/internal/platform/config":       true,
		modulePath + "/internal/platform/rpc":          true,
	}
)

// PackageCoverage aggregates one package's coverage.
type PackageCoverage struct {
	Path         string // full package import path
	TotalStmts   int    // sum of numStmts across all file lines
	CoveredStmts int    // sum where count > 0
}

// Percent returns covered / total * 100, or 0 if TotalStmts == 0.
func (p PackageCoverage) Percent() float64 {
	if p.TotalStmts == 0 {
		return 0
	}
	return float64(p.CoveredStmts) / float64(p.TotalStmts) * 100
}

// Tier classifies a package per spec D-1.
type Tier int

// Tier values returned by classify().
const (
	TierCore   Tier = iota // ≥ 85%
	TierTool               // ≥ 90% (T-13a)
	TierOther              // ≥ 75%
	TierExempt             // no threshold
)

// String returns the lowercase tier name used in violation output.
func (t Tier) String() string {
	switch t {
	case TierCore:
		return "core"
	case TierTool:
		return "tool"
	case TierOther:
		return "other"
	case TierExempt:
		return "exempt"
	default:
		return "unknown"
	}
}

func classify(pkg string) Tier {
	if exemptPackages[pkg] {
		return TierExempt
	}
	if corePackages[pkg] {
		return TierCore
	}
	if toolPackages[pkg] {
		return TierTool
	}
	return TierOther
}

func threshold(tier Tier) float64 {
	switch tier {
	case TierCore:
		return coreThreshold
	case TierTool:
		return toolThreshold
	case TierOther:
		return otherThreshold
	default:
		return 0 // exempt has no threshold
	}
}

// Violation describes a package below its threshold.
type Violation struct {
	Package   string
	Tier      Tier
	Percent   float64
	Threshold float64
}

// String renders a violation for stderr (indented; one per line).
func (v Violation) String() string {
	return fmt.Sprintf("  [%s] %s: %.1f%% (< %.0f%%)",
		v.Tier, v.Package, v.Percent, v.Threshold)
}

// ParseProfile reads a `go test -coverprofile` file and returns a map of
// package path → aggregated coverage. Lines are:
//
//	mode: set        (first line, ignored)
//	<pkg/file>.go:<startLine>.<col>,<endLine>.<col> <numStmts> <count>
//
// We strip the trailing /<filename>.go to get the package import path.
func ParseProfile(path string) (map[string]*PackageCoverage, error) {
	f, err := os.Open(path) // #nosec G304 -- spec-0.9 D-2: operator-supplied lint tool path
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	pkgs := make(map[string]*PackageCoverage)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if lineNum == 1 && strings.HasPrefix(line, "mode:") {
			continue
		}
		if line == "" {
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			return nil, fmt.Errorf("line %d: missing ':' in %q", lineNum, line)
		}
		fileSpec := line[:colon]
		slash := strings.LastIndex(fileSpec, "/")
		if slash < 0 {
			return nil, fmt.Errorf("line %d: missing '/' in package path %q", lineNum, fileSpec)
		}
		pkgPath := fileSpec[:slash]

		rest := strings.TrimSpace(line[colon+1:])
		fields := strings.Fields(rest)
		if len(fields) < 3 {
			return nil, fmt.Errorf("line %d: expected 3 fields after colon, got %d", lineNum, len(fields))
		}
		numStmts, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("line %d: numStmts not integer: %w", lineNum, err)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("line %d: count not integer: %w", lineNum, err)
		}

		p, ok := pkgs[pkgPath]
		if !ok {
			p = &PackageCoverage{Path: pkgPath}
			pkgs[pkgPath] = p
		}
		p.TotalStmts += numStmts
		if count > 0 {
			p.CoveredStmts += numStmts
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return pkgs, nil
}

// ListPackages invokes `go list ./...` in the current working directory
// and returns the resulting import paths. T-13a codex MED-3 feeder for
// InjectMissing.
func ListPackages() ([]string, error) {
	cmd := exec.Command("go", "list", "./...")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

// InjectMissing adds zero-coverage entries for non-exempt packages in
// allPackages that are absent from pkgs. T-13a codex MED-3: a package
// with code but no test files is otherwise invisible to the gate.
func InjectMissing(pkgs map[string]*PackageCoverage, allPackages []string) {
	for _, p := range allPackages {
		if _, exists := pkgs[p]; exists {
			continue
		}
		if classify(p) == TierExempt {
			continue
		}
		pkgs[p] = &PackageCoverage{Path: p}
	}
}

// Check returns violations + (totalPct, totalThresholdMet) given parsed
// package coverages. exemptPackages are excluded from the total calculation.
func Check(pkgs map[string]*PackageCoverage) (violations []Violation, totalPct float64, totalOK bool) {
	var totalStmts, totalCovered int

	paths := make([]string, 0, len(pkgs))
	for p := range pkgs {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, path := range paths {
		p := pkgs[path]
		tier := classify(path)
		pct := p.Percent()
		thr := threshold(tier)

		if tier != TierExempt {
			totalStmts += p.TotalStmts
			totalCovered += p.CoveredStmts
			if pct < thr {
				violations = append(violations, Violation{
					Package:   path,
					Tier:      tier,
					Percent:   pct,
					Threshold: thr,
				})
			}
		}
	}

	if totalStmts > 0 {
		totalPct = float64(totalCovered) / float64(totalStmts) * 100
	}
	totalOK = totalPct >= totalThreshold
	return violations, totalPct, totalOK
}

// run executes the gate logic and returns the desired exit code. Split
// from main() for test coverage (T-13a). w receives stderr-equivalent
// output; lister enumerates packages for missing-injection (nil = skip).
func run(profilePath string, verbose bool, w io.Writer, lister func() ([]string, error)) int {
	if os.Getenv("COVERAGE_GATE_SKIP") == "1" {
		_, _ = fmt.Fprintln(w, "==============================================================")
		_, _ = fmt.Fprintln(w, "WARNING: COVERAGE_GATE_SKIP=1 — coverage threshold check bypassed")
		_, _ = fmt.Fprintln(w, "         (emergency override; CHANGELOG must note usage)")
		_, _ = fmt.Fprintln(w, "==============================================================")
		return 0
	}

	pkgs, err := ParseProfile(profilePath)
	if err != nil {
		_, _ = fmt.Fprintf(w, "coverage-gate: parse %s: %v\n", profilePath, err)
		return 2
	}

	if lister != nil {
		all, lerr := lister()
		if lerr != nil {
			_, _ = fmt.Fprintf(w, "coverage-gate: go list failed (continuing without missing-injection): %v\n", lerr)
		} else {
			InjectMissing(pkgs, all)
		}
	}

	if verbose {
		paths := make([]string, 0, len(pkgs))
		for p := range pkgs {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		_, _ = fmt.Fprintln(w, "coverage-gate per-package report:")
		for _, p := range paths {
			pc := pkgs[p]
			_, _ = fmt.Fprintf(w, "  [%s] %s: %.1f%% (%d/%d)\n",
				classify(p), p, pc.Percent(), pc.CoveredStmts, pc.TotalStmts)
		}
	}

	violations, totalPct, totalOK := Check(pkgs)

	if len(violations) == 0 && totalOK {
		_, _ = fmt.Fprintf(w, "coverage-gate OK (total %.1f%% ≥ %.0f%%; %d packages checked)\n",
			totalPct, totalThreshold, len(pkgs))
		return 0
	}

	_, _ = fmt.Fprintf(w, "coverage-gate FAIL\n")
	if len(violations) > 0 {
		_, _ = fmt.Fprintf(w, "  per-package violations (%d):\n", len(violations))
		for _, v := range violations {
			_, _ = fmt.Fprintln(w, v)
		}
	}
	if !totalOK {
		_, _ = fmt.Fprintf(w, "  total coverage %.1f%% < %.0f%% threshold\n", totalPct, totalThreshold)
	}
	_, _ = fmt.Fprintln(w, "  hint: see CLAUDE.md 规则 8 + spec-0.8 D-1 for tier definitions")
	_, _ = fmt.Fprintln(w, "  emergency bypass: COVERAGE_GATE_SKIP=1 (note in CHANGELOG)")
	return 1
}

func main() {
	profilePath := flag.String("profile", "coverage.out", "path to `go test -coverprofile` output")
	verbose := flag.Bool("v", false, "verbose: print all packages with their tier + coverage")
	// T-13a codex MED-3 — InjectMissing capability is opt-in via -enumerate.
	// Off by default because Stage 0 has many `doc.go`-only scaffold packages
	// (internal/app/services/* / internal/domain/* — Stage 1+) that would
	// false-positive as 0%-coverage violations. Full enforcement deferred to
	// the spec that introduces real code in those packages (spec-1.X+).
	enumerate := flag.Bool("enumerate", false, "use `go list ./...` to inject 0% for non-exempt packages missing from profile")
	flag.Parse()

	var lister func() ([]string, error)
	if *enumerate {
		lister = ListPackages
	}
	os.Exit(run(*profilePath, *verbose, os.Stderr, lister))
}
