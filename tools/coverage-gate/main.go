// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// coverage-gate enforces CLAUDE.md 规则 8 per-package coverage thresholds
// against a `go test -coverprofile=...` output file.
//
// spec-0.8 D-1 / T-3.
//
// Tiers (R2 用户拍板 CRIT-A):
//
//   - Core packages (≥ 85%):
//       internal/platform/{errcode, logger, version}
//   - Other packages (≥ 75%): everything not in core/exempt
//   - Exempt (no threshold): entrypoints stub + tools/* lint tools +
//     cmd/opendbx + internal/platform/config + internal/platform/rpc
//     (tech debt; spec-1.X UI 实施后单独升 core)
//   - Total project (≥ 80%): aggregated across all non-exempt packages
//
// Profile parsing (claude T-12 H1 R2 修): `go tool cover -func` only emits
// a single project-wide `total:` line — no per-package totals. We must
// parse the raw profile format ourselves:
//
//	mode: <set|count|atomic>
//	<pkg/file>:<startLine>.<col>,<endLine>.<col> <numStmts> <count>
//	...
//
// Per-package coverage = sum(numStmts where count > 0) / sum(numStmts),
// grouped by stripping the trailing `/<file>.go` portion of `pkg/file`.
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
	"os"
	"sort"
	"strconv"
	"strings"
)

// Tiers per spec-0.8 D-1.
const (
	coreThreshold  = 85.0
	otherThreshold = 75.0
	totalThreshold = 80.0
)

// modulePath prefix for grouping packages from profile entries.
const modulePath = "github.com/sqlrush/opendbx"

//nolint:gochecknoglobals // configuration constants by design.
var (
	corePackages = map[string]bool{
		modulePath + "/internal/platform/errcode": true,
		modulePath + "/internal/platform/logger":  true,
		modulePath + "/internal/platform/version": true,
	}

	exemptPackages = map[string]bool{
		modulePath + "/internal/entrypoints":           true,
		modulePath + "/tools/import-rules-check":       true,
		modulePath + "/tools/import-rules-check/rules": true,
		modulePath + "/tools/dep-allowlist-check":      true,
		modulePath + "/tools/coverage-gate":            true, // self-exempt: assertion logic
		modulePath + "/tools/makefile-check":           true, // spec-0.8 D-6 (lands in T-9)
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

const (
	TierCore Tier = iota
	TierOther
	TierExempt
)

func (t Tier) String() string {
	switch t {
	case TierCore:
		return "core"
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
	return TierOther
}

func threshold(tier Tier) float64 {
	switch tier {
	case TierCore:
		return coreThreshold
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
	f, err := os.Open(path) //nolint:gosec // operator-supplied lint tool path
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
		// Format: <pkg/file>:<startLine>.<col>,<endLine>.<col> <numStmts> <count>
		colon := strings.Index(line, ":")
		if colon < 0 {
			return nil, fmt.Errorf("line %d: missing ':' in %q", lineNum, line)
		}
		fileSpec := line[:colon]
		// Strip trailing /<filename>.go
		slash := strings.LastIndex(fileSpec, "/")
		if slash < 0 {
			return nil, fmt.Errorf("line %d: missing '/' in package path %q", lineNum, fileSpec)
		}
		pkgPath := fileSpec[:slash]

		// Parse the trailing "<numStmts> <count>" portion.
		rest := strings.TrimSpace(line[colon+1:])
		fields := strings.Fields(rest)
		if len(fields) < 3 {
			return nil, fmt.Errorf("line %d: expected 3 fields after colon, got %d", lineNum, len(fields))
		}
		// fields[0] = "<start>.<col>,<end>.<col>" (range; we don't need it)
		numStmts, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("line %d: numStmts not integer: %v", lineNum, err)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("line %d: count not integer: %v", lineNum, err)
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

// Check returns violations + (totalPct, totalThresholdMet) given parsed
// package coverages. exemptPackages are excluded from the total calculation.
func Check(pkgs map[string]*PackageCoverage) (violations []Violation, totalPct float64, totalOK bool) {
	var totalStmts, totalCovered int

	// Sort packages for deterministic output.
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

func main() {
	profilePath := flag.String("profile", "coverage.out", "path to `go test -coverprofile` output")
	verbose := flag.Bool("v", false, "verbose: print all packages with their tier + coverage")
	flag.Parse()

	// Q11 ★A: emergency override env. Loud warning to stderr; exit 0.
	if os.Getenv("COVERAGE_GATE_SKIP") == "1" {
		fmt.Fprintln(os.Stderr, "==============================================================")
		fmt.Fprintln(os.Stderr, "WARNING: COVERAGE_GATE_SKIP=1 — coverage threshold check bypassed")
		fmt.Fprintln(os.Stderr, "         (emergency override; CHANGELOG must note usage)")
		fmt.Fprintln(os.Stderr, "==============================================================")
		os.Exit(0)
	}

	pkgs, err := ParseProfile(*profilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "coverage-gate: parse %s: %v\n", *profilePath, err)
		os.Exit(2)
	}

	if *verbose {
		paths := make([]string, 0, len(pkgs))
		for p := range pkgs {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		fmt.Fprintln(os.Stderr, "coverage-gate per-package report:")
		for _, p := range paths {
			pc := pkgs[p]
			fmt.Fprintf(os.Stderr, "  [%s] %s: %.1f%% (%d/%d)\n",
				classify(p), p, pc.Percent(), pc.CoveredStmts, pc.TotalStmts)
		}
	}

	violations, totalPct, totalOK := Check(pkgs)

	if len(violations) == 0 && totalOK {
		fmt.Fprintf(os.Stderr, "coverage-gate OK (total %.1f%% ≥ %.0f%%; %d packages checked)\n",
			totalPct, totalThreshold, len(pkgs))
		os.Exit(0)
	}

	fmt.Fprintf(os.Stderr, "coverage-gate FAIL\n")
	if len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "  per-package violations (%d):\n", len(violations))
		for _, v := range violations {
			fmt.Fprintln(os.Stderr, v)
		}
	}
	if !totalOK {
		fmt.Fprintf(os.Stderr, "  total coverage %.1f%% < %.0f%% threshold\n", totalPct, totalThreshold)
	}
	fmt.Fprintln(os.Stderr, "  hint: see CLAUDE.md 规则 8 + spec-0.8 D-1 for tier definitions")
	fmt.Fprintln(os.Stderr, "  emergency bypass: COVERAGE_GATE_SKIP=1 (note in CHANGELOG)")
	os.Exit(1)
}
