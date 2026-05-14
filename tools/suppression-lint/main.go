// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package main implements suppression-lint, a custom checker enforcing the
// opendbx suppression-comment spec_ref protocol (spec-0.10 D-2.5 / T-3.5).
//
// Why this exists (R2 codex H11):
//
// golangci-lint's nolintlint cannot enforce a custom `-- spec-X.Y D-N:`
// suffix format, only that a `//nolint` directive carries SOME rationale.
// We also have multiple suppression families (`// #nosec`, `// errcode-
// lint:exempt`, `// govulncheck-exempt`) with no unified policy.
// suppression-lint scans 4 families and fails when any comment lacks a
// `spec-X.Y(.Z)?(-tN)?` reference.
//
// Families (spec-0.10 § D-6 table):
//
//   - //nolint:<linter> ...               (golangci-lint)
//   - // #nosec [G-id]                    (gosec)
//   - // errcode-lint:exempt              (custom; spec-0.10 D-2)
//   - // govulncheck-exempt:<osv-id>      (govulncheck wrapper; mostly in JSON allowlist,
//     which is checked separately via spec_ref field)
//
// Each suppression line MUST contain a `spec-N.M[.X][-tN]` token
// (e.g. `spec-0.9 D-2` / `spec-0.6 § 2.2.1`).
//
// Usage:
//
//	go run ./tools/suppression-lint [-v] [path...]
//
// Default path is `.` (current directory). Walks all *.go files under
// path, skipping vendor/, testdata/, and _test.go files by default.
//
// Exit codes:
//
//	0  all suppression comments carry spec_ref
//	1  one or more violations
//	2  I/O error
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// suppressionRE matches the start of any of the 4 family prefixes.
// Captures the whole line content from the comment onwards for spec_ref scan.
var suppressionRE = regexp.MustCompile(
	`(//\s*nolint:[a-zA-Z0-9_,-]+\b.*$|//\s*#nosec\b.*$|//\s*errcode-lint:exempt\b.*$|//\s*govulncheck-exempt:.*$)`)

// specRefRE matches a spec reference token: spec-N.M, optional letter
// suffix (e.g. 0.15a), optional .X minor, optional -tN task id.
// Examples that match:
//
//	spec-0.9 D-2
//	spec-0.6 § 2.2.1
//	spec-0.15a
//	spec-0.8-t10a
//	spec-0.10 D-2.5
var specRefRE = regexp.MustCompile(`\bspec-[0-9]+\.[0-9]+[a-z]?(\.[0-9]+)?(-t[0-9]+[a-z]?)?\b`)

// Family identifies which suppression family a comment belongs to.
type Family int

// Suppression family tags.
const (
	FamilyNolint Family = iota
	FamilyNosec
	FamilyErrcodeLint
	FamilyGovulncheck
)

// String returns the lowercase family name used in violation output.
func (f Family) String() string {
	switch f {
	case FamilyNolint:
		return "nolint"
	case FamilyNosec:
		return "nosec"
	case FamilyErrcodeLint:
		return "errcode-lint"
	case FamilyGovulncheck:
		return "govulncheck-exempt"
	default:
		return "unknown"
	}
}

// classify identifies which family a matched suppression belongs to.
func classify(comment string) Family {
	switch {
	case strings.Contains(comment, "//nolint:") || strings.Contains(comment, "// nolint:"):
		return FamilyNolint
	case strings.Contains(comment, "#nosec"):
		return FamilyNosec
	case strings.Contains(comment, "errcode-lint:exempt"):
		return FamilyErrcodeLint
	case strings.Contains(comment, "govulncheck-exempt"):
		return FamilyGovulncheck
	default:
		return Family(-1)
	}
}

// Violation describes one missing-spec_ref suppression comment.
type Violation struct {
	File    string
	Line    int
	Family  Family
	Comment string
}

// String renders a violation for stderr (indented; one per line).
func (v Violation) String() string {
	return fmt.Sprintf("  [%s] %s:%d — missing spec_ref token\n    > %s",
		v.Family, v.File, v.Line, strings.TrimSpace(v.Comment))
}

// scanFile scans a single Go file for suppression comments lacking spec_ref.
// Returns the violations found (possibly empty) and any I/O error.
func scanFile(path string) ([]Violation, error) {
	f, err := os.Open(path) // #nosec G304 -- spec-0.10 D-2.5: operator-supplied path
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var violations []Violation
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		m := suppressionRE.FindString(line)
		if m == "" {
			continue
		}
		if specRefRE.MatchString(m) {
			continue // spec_ref present, OK
		}
		fam := classify(m)
		if fam == Family(-1) {
			continue
		}
		violations = append(violations, Violation{
			File:    path,
			Line:    lineNum,
			Family:  fam,
			Comment: line,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return violations, nil
}

// shouldSkip returns true for paths that suppression-lint must not scan.
// Skips: vendor/, testdata/ subtrees; non-.go files; _test.go files; hidden
// directories (e.g. .git).
func shouldSkip(path string, d fs.DirEntry) bool {
	name := d.Name()
	if d.IsDir() {
		return name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".")
	}
	if !strings.HasSuffix(name, ".go") {
		return true
	}
	if strings.HasSuffix(name, "_test.go") {
		return true
	}
	return false
}

// scanTree walks root recursively and returns all suppression violations.
func scanTree(root string) ([]Violation, error) {
	var all []Violation
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if shouldSkip(path, d) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil // walk into; don't try to scan directory as file
		}
		vs, err := scanFile(path)
		if err != nil {
			return err
		}
		all = append(all, vs...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		return all[i].Line < all[j].Line
	})
	return all, nil
}

// run is the testable entry point.
func run(paths []string, verbose bool, w fileWriter) int {
	var all []Violation
	for _, p := range paths {
		vs, err := scanTree(p)
		if err != nil {
			_, _ = fmt.Fprintf(w, "suppression-lint: %v\n", err)
			return 2
		}
		all = append(all, vs...)
	}
	if verbose {
		_, _ = fmt.Fprintf(w, "suppression-lint: scanned %d path(s); %d violation(s)\n", len(paths), len(all))
	}
	if len(all) == 0 {
		_, _ = fmt.Fprintln(w, "suppression-lint OK: all suppression comments carry spec_ref")
		return 0
	}
	_, _ = fmt.Fprintf(w, "suppression-lint FAIL: %d violation(s)\n", len(all))
	for _, v := range all {
		_, _ = fmt.Fprintln(w, v)
	}
	_, _ = fmt.Fprintln(w, "hint: each //nolint, // #nosec, // errcode-lint:exempt, // govulncheck-exempt")
	_, _ = fmt.Fprintln(w, "      comment must reference a spec via `spec-N.M[.X][-tN]` token, e.g.")
	_, _ = fmt.Fprintln(w, "      `//nolint:gochecknoglobals // spec-0.6 § 2.2.1 canonical Register pattern`")
	return 1
}

// fileWriter is the minimal output interface used by run() for testability.
type fileWriter interface {
	Write(p []byte) (n int, err error)
}

// realMain wraps flag parsing + default-path logic so main() itself is a
// 1-line shim that test coverage tools can ignore.
func realMain(args []string, w fileWriter) int {
	fs := flag.NewFlagSet("suppression-lint", flag.ContinueOnError)
	fs.SetOutput(w)
	verbose := fs.Bool("v", false, "verbose: print summary even when OK")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}
	return run(paths, *verbose, w)
}

func main() {
	os.Exit(realMain(os.Args[1:], os.Stderr))
}
