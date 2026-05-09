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
func scan(root string) ([]string, int, error) {
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedImports,
		Dir:   root,
		Tests: false,
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
	for _, pkg := range pkgs {
		// Only consider packages from the opendbx module (skip stdlib + transitive
		// external deps that get pulled into the package graph).
		if !strings.HasPrefix(pkg.PkgPath, rules.ModulePrefix) {
			continue
		}
		scanned++
		for _, imp := range pkg.Imports {
			edges := checkEdge(pkg.PkgPath, imp.PkgPath)
			violations = append(violations, edges...)
		}
	}
	return violations, scanned, nil
}

// checkEdge runs the three rule families against a single edge.
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
	return out
}
