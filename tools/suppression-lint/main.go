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
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// directiveRE matches the prefix of a suppression directive when it
// appears at the very START of a comment's textual content (after the
// leading `//` slashes and optional whitespace). T-13 go-reviewer MED-2
// fix: previous suppressionRE matched substrings anywhere on the line,
// producing false positives inside string literals and doc-comment
// references. We now parse Go source via go/parser and inspect only
// *ast.Comment nodes; this regex applies to the comment's *Text*
// without its leading `//` or `/*` markers.
var directiveRE = regexp.MustCompile(
	`^(nolint:[a-zA-Z0-9_,-]+|#nosec\b|errcode-lint:exempt|govulncheck-exempt:)`)

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

// classify identifies which family a comment's normalized text belongs
// to. The text must already have its `//` or `/*` prefix and leading
// whitespace stripped. T-13 go-reviewer MED-2 fix: only directives at the
// start of a comment qualify, eliminating string-literal false matches.
func classify(text string) Family {
	switch {
	case strings.HasPrefix(text, "nolint:"):
		return FamilyNolint
	case strings.HasPrefix(text, "#nosec"):
		return FamilyNosec
	case strings.HasPrefix(text, "errcode-lint:exempt"):
		return FamilyErrcodeLint
	case strings.HasPrefix(text, "govulncheck-exempt:"):
		return FamilyGovulncheck
	default:
		return Family(-1)
	}
}

// stripCommentMarker strips `//` or `/* ... */` markers and surrounding
// whitespace from a Go comment's Text field, returning the bare comment
// body suitable for directive matching.
func stripCommentMarker(text string) string {
	if strings.HasPrefix(text, "//") {
		return strings.TrimSpace(text[2:])
	}
	if strings.HasPrefix(text, "/*") && strings.HasSuffix(text, "*/") {
		return strings.TrimSpace(text[2 : len(text)-2])
	}
	return strings.TrimSpace(text)
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

// scanFile parses a single Go file via go/parser and inspects its comment
// nodes for suppression directives lacking spec_ref. T-13 go-reviewer
// MED-2 fix: AST-based scanning eliminates false positives that the
// previous line-regex variant produced for directive-shaped substrings
// inside string literals and doc-comment references.
func scanFile(path string) ([]Violation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var violations []Violation
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			body := stripCommentMarker(c.Text)
			if !directiveRE.MatchString(body) {
				continue
			}
			fam := classify(body)
			if fam == Family(-1) {
				continue
			}
			if specRefRE.MatchString(body) {
				continue // spec_ref present, OK
			}
			pos := fset.Position(c.Pos())
			violations = append(violations, Violation{
				File:    path,
				Line:    pos.Line,
				Family:  fam,
				Comment: c.Text,
			})
		}
	}
	return violations, nil
}

// shouldSkip returns true for paths that suppression-lint must not scan.
// Skips: vendor/, testdata/ subtrees; non-.go files; _test.go files; hidden
// directories (e.g. .git).
//
// T-13 go-reviewer CRIT-1: the root entry of WalkDir has d.Name()=="." or
// "..", which previously triggered the "hidden directory" heuristic and
// silently aborted the entire walk. We exempt the root explicitly via
// `isRoot` (path matches one of the user-supplied roots) — the heuristic
// only applies to deeper directories.
func shouldSkip(path string, d fs.DirEntry, isRoot bool) bool {
	if isRoot {
		// Never skip the user-supplied root, even if its leaf name is "."
		// or starts with ".".
		return false
	}
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
		isRoot := path == root
		if shouldSkip(path, d, isRoot) {
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
