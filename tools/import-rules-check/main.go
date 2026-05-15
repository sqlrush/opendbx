// Copyright 2026 opendbx contributors. See LICENSE.
//
// import-rules-check: validates Go import edges in the opendbx module
// against spec-0.2 § 2.2 (layer matrix + cluster restrictions + render DAG).
//
// Usage:
//
//	import-rules-check [-v] [<repo-root>]
//
// Default <repo-root> is the working directory. The tool walks the entire
// opendbx module via golang.org/x/tools/go/packages, classifies each import
// edge, and exits 1 with a list of violations on the first rule each edge
// trips. Stdlib edges and external (non-opendbx) edges are skipped — those
// are dep-allowlist-check's responsibility.
//
// Author: sqlrush
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/sqlrush/opendbx/tools/import-rules-check/rules"
	"golang.org/x/tools/go/packages"
)

const usage = `import-rules-check [-v] [<repo-root>]

Validates opendbx import edges per spec-0.2 § 2.2.

Flags:
  -v        verbose (print package count on success)
  <root>    opendbx repo root (default: current directory)
`

func main() {
	var verbose bool
	flag.BoolVar(&verbose, "v", false, "verbose")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	root := flag.Arg(0)
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, "getwd:", err)
			os.Exit(2)
		}
	}

	violations, pkgCount, err := scan(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "import-rules-check:", err)
		os.Exit(2)
	}

	if len(violations) > 0 {
		fmt.Fprintln(os.Stderr, "import-rules-check FAIL:")
		sort.Strings(violations)
		for _, v := range violations {
			fmt.Fprintln(os.Stderr, "  -", v)
		}
		os.Exit(1)
	}
	if verbose {
		fmt.Printf("import-rules-check OK (%d packages scanned)\n", pkgCount)
	}
}

// scan loads all packages under root and returns the list of violation
// strings (sorted later by caller). Returns the count of opendbx-internal
// packages scanned for the verbose mode.
//
// Tests:true ensures *_test.go files participate in the import graph; the
// test variant of a package gets a synthesized PkgPath like
// `<pkg>.test` or `<pkg> [<pkg>.test]` — strip those before classifying.
func scan(root string) ([]string, int, error) {
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedImports,
		Dir:   root,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, 0, fmt.Errorf("packages.Load: %w", err)
	}

	var loadErrs []string
	for _, p := range pkgs {
		for _, e := range p.Errors {
			loadErrs = append(loadErrs, fmt.Sprintf("%s: %s", p.PkgPath, e))
		}
	}
	if len(loadErrs) > 0 {
		return nil, 0, fmt.Errorf("packages.Load returned errors:\n  %s", strings.Join(loadErrs, "\n  "))
	}

	var violations []string
	scanned := 0
	seen := make(map[string]bool, len(pkgs))
	for _, pkg := range pkgs {
		fromPath := canonicalPath(pkg.PkgPath)
		// Only consider packages from the opendbx module (skip stdlib + transitive
		// external deps that get pulled into the package graph).
		if !strings.HasPrefix(fromPath, rules.ModulePrefix) {
			continue
		}
		// Avoid double-counting: Tests:true emits the same package twice
		// (production + test variant). Count distinct PkgPaths only.
		if !seen[fromPath] {
			seen[fromPath] = true
			scanned++
		}
		fromIsTestVariant := isTestVariant(pkg.PkgPath)
		for _, imp := range pkg.Imports {
			toPath := canonicalPath(imp.PkgPath)
			// Skip self-edges (Tests:true synthesizes test variant of a
			// package that imports itself; not a real Go import edge).
			if fromPath == toPath {
				continue
			}
			// Test-variant edges into internal/testing/* are always allowed:
			// the production package itself doesn't gain a dependency on test
			// helpers; only its _test.go files do. spec-0.11 T-13 codex HIGH-3.
			if fromIsTestVariant && strings.HasPrefix(toPath, rules.ModulePrefix+"internal/testing/") {
				continue
			}
			edges := checkEdge(fromPath, toPath)
			// spec-0.12 R3 H-4 + R-13: IMP-9 tcell-isolation exempts test
			// files. Test-variant edges importing tcell from a non-whitelisted
			// package (e.g. cmd/opendbx/root_test.go hook signature) are OK.
			if fromIsTestVariant {
				edges = filterTcellViolations(edges)
			}
			violations = append(violations, edges...)
		}
	}

	// Filesystem pass: doc.go presence + pkg/ empty.
	fsViolations, err := rules.CheckFilesystem(root)
	if err != nil {
		return nil, 0, fmt.Errorf("filesystem check: %w", err)
	}
	violations = append(violations, fsViolations...)

	return violations, scanned, nil
}

// canonicalPath strips the test-variant suffix that packages.Load adds when
// Tests:true (e.g. `pkg.test`, `pkg [pkg.test]`).
func canonicalPath(p string) string {
	if i := strings.Index(p, " ["); i >= 0 {
		return p[:i]
	}
	return strings.TrimSuffix(p, ".test")
}

// filterTcellViolations drops IMP-9 violations from the edge list.
// Used for test-variant edges where importing tcell.Screen for hook
// signatures is acceptable (spec-0.12 R3 H-4 + R-13).
func filterTcellViolations(edges []string) []string {
	out := edges[:0]
	for _, e := range edges {
		if !strings.Contains(e, "IMP-9 tcell-isolation") {
			out = append(out, e)
		}
	}
	return out
}

// isTestVariant reports whether p is the synthesized test variant of a
// package (so its imports include _test.go files only). spec-0.11 T-13
// codex HIGH-3: needed to grant test-only edges to internal/testing/*.
func isTestVariant(p string) bool {
	return strings.HasSuffix(p, ".test") || strings.Contains(p, ".test]")
}

// checkEdge runs every rule family against a single edge.
//
// Rule families (existing + spec-0.10 D-3 IMP-5/6/7/8 + spec-0.12 IMP-9):
//   - CheckEdge:        layer matrix (spec-0.2 D-5)
//   - CheckCluster:     domain cluster restrictions (spec-0.2 D-5)
//   - CheckRenderDAG:   render/* 10-layer strict DAG (spec-0.2 D-5; IMP-6 per spec-0.10)
//   - CheckOpendbBan:   IMP-5 (spec-0.10 D-3)
//   - CheckLLMSDKIsolation: IMP-7 (spec-0.10 D-3, added T-7)
//   - CheckRunewidthWrap:   IMP-8 (spec-0.10 D-3, added T-7)
//   - CheckTcellIsolation:  IMP-9 (spec-0.12 D-1)
func checkEdge(from, to string) []string {
	var out []string
	if r := rules.CheckEdge(from, to); r != "" {
		out = append(out, fmt.Sprintf("%s → %s: %s", from, to, r))
	}
	if r := rules.CheckCluster(from, to); r != "" {
		out = append(out, fmt.Sprintf("%s → %s: %s", from, to, r))
	}
	if r := rules.CheckRenderDAG(from, to); r != "" {
		out = append(out, fmt.Sprintf("%s → %s: %s", from, to, r))
	}
	if r := rules.CheckOpendbBan(from, to); r != "" {
		out = append(out, fmt.Sprintf("%s → %s: %s", from, to, r))
	}
	if r := rules.CheckLLMSDKIsolation(from, to); r != "" {
		out = append(out, fmt.Sprintf("%s → %s: %s", from, to, r))
	}
	if r := rules.CheckRunewidthWrap(from, to); r != "" {
		out = append(out, fmt.Sprintf("%s → %s: %s", from, to, r))
	}
	if r := rules.CheckTcellIsolation(from, to); r != "" {
		out = append(out, fmt.Sprintf("%s → %s: %s", from, to, r))
	}
	return out
}
