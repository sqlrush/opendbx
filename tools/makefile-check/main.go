// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package main implements makefile-check, a lint tool that enforces the
// opendbx + opendbrb Makefile conventions (spec-0.8 D-6 / T-9).
//
// Checks (5 rules):
//
//  1. Every rule target has a `## help-text` doc comment on its definition
//     line (consumed by `make help`).
//  2. Every non-pattern target appears in some `.PHONY:` line (multiple
//     .PHONY lines are unioned; line-continuation `\` NOT supported per
//     spec § 2.3 — recipes must use single-line `.PHONY:` form).
//  3. Target names match `lower-kebab-case`: `^[a-z][a-z0-9-]*$`.
//  4. No duplicate target names.
//  5. Doc-block (spec-0.8 D-7 binary criterion): the file begins with a
//     comment block (lines starting `#`) containing:
//     - ≥ 3 distinct category headings (e.g., "用户日常", "CI", "release", "维护")
//     - ≥ 1 line mentioning a cross-repo path (`../opendbx` or `../opendbrb`)
//     - ≥ 1 line mentioning GNU make / bash / shell requirements
//
// Scope (R2 claude MED-1 fix):
//   - Top-level rule definitions only.
//   - Ignores pattern rules (`%:`), conditional blocks (ifeq/ifdef/endif),
//     and include directives.
//   - Single-line `.PHONY:` only (continuation `\` rejected as MED-1
//     mandate — keeps the checker simple and the Makefiles consistent).
//
// Usage:
//
//	go run ./tools/makefile-check Makefile [Makefile2 ...]
//	go run ./tools/makefile-check -v Makefile     # verbose: list every parsed target
//
// Exit codes:
//
//	0  all files pass
//	1  ≥ 1 violation
//	3  internal error (read failure)
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Compiled patterns. Anchored to start-of-line.
var (
	// Top-level rule definition. Matches `name: deps ## help`.
	// Group 1: target name. Group 2: help text. Captures only if `##` exists.
	ruleRE = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_.-]*):.*$`)

	// Rule with help comment captured.
	ruleWithHelpRE = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_.-]*):.*##\s+(.+)$`)

	// .PHONY directive.
	phonyRE = regexp.MustCompile(`^\.PHONY:\s+(.+)$`)

	// kebab-lower target name (production rules).
	kebabLowerRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

	// Pattern rule (e.g., `%.o: %.c`) — skipped from checks.
	patternRE = regexp.MustCompile(`^[A-Za-z0-9_.-]*%`)

	// Conditional / include directives — skipped.
	conditionalRE = regexp.MustCompile(`^(ifeq|ifneq|ifdef|ifndef|else|endif|include|-include|sinclude)\b`)

	// Variable / assignment line — skipped.
	assignRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\s*[:?+]?=`)

	// Line continuation rejection (MED-1: only single-line .PHONY allowed).
	continuationRE = regexp.MustCompile(`\\\s*$`)
)

// VKind enumerates the 5 violation classes.
type VKind string

// Violation kind tags emitted in output.
const (
	VMissingHelp     VKind = "missing-help-comment"
	VPhonyMissing    VKind = "phony-missing-target"
	VNameNotKebab    VKind = "name-not-kebab-lower"
	VDuplicateTarget VKind = "duplicate-target"
	VDocBlock        VKind = "doc-block-incomplete"
	VPhonyContinue   VKind = "phony-line-continuation" // MED-1: rejected
)

// Violation describes one finding.
type Violation struct {
	File    string
	Line    int
	Kind    VKind
	Target  string // empty for VDocBlock
	Message string
}

func (v Violation) String() string {
	loc := v.File
	if v.Line > 0 {
		loc = fmt.Sprintf("%s:%d", v.File, v.Line)
	}
	target := ""
	if v.Target != "" {
		target = fmt.Sprintf(" target=%q", v.Target)
	}
	return fmt.Sprintf("  [%s] %s%s — %s", v.Kind, loc, target, v.Message)
}

// targetInfo tracks where a target was defined.
type targetInfo struct {
	Line int
	Help string // empty = no ## comment
}

// Check parses path as a Makefile and returns all violations found.
func Check(path string) ([]Violation, error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied lint tool path
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var (
		violations []Violation
		targets    = map[string]*targetInfo{}
		phony      = map[string]bool{}
		docLines   []string // comment lines at the very top of the file
		lineNum    int
		seenCode   bool // toggle when we leave the top doc block
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		line := raw
		trim := strings.TrimSpace(line)

		// Top-of-file doc block: consecutive comment lines until first
		// non-comment, non-blank line.
		if !seenCode {
			switch {
			case strings.HasPrefix(trim, "#"):
				docLines = append(docLines, trim)
				continue
			case trim == "":
				continue
			default:
				seenCode = true
			}
		}

		// Skip recipe lines (indented with TAB).
		if strings.HasPrefix(raw, "\t") {
			continue
		}
		// Skip empty / comment lines outside doc block.
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		// Skip conditionals / include.
		if conditionalRE.MatchString(trim) {
			continue
		}
		// .PHONY directive.
		if m := phonyRE.FindStringSubmatch(trim); m != nil {
			// MED-1: reject continuation.
			if continuationRE.MatchString(raw) {
				violations = append(violations, Violation{
					File: path, Line: lineNum, Kind: VPhonyContinue,
					Message: ".PHONY uses line continuation '\\'; single-line form required (spec § 2.3)",
				})
				continue
			}
			for _, name := range strings.Fields(m[1]) {
				phony[name] = true
			}
			continue
		}
		// Skip pattern rules.
		if patternRE.MatchString(trim) {
			continue
		}
		// Skip variable assignments. (Must come BEFORE ruleRE since `FOO :=`
		// has colon-equals; ruleRE would match the bare `FOO:` prefix.)
		if assignRE.MatchString(trim) {
			continue
		}
		// Rule definition.
		if m := ruleRE.FindStringSubmatch(trim); m != nil {
			name := m[1]
			if prev, dup := targets[name]; dup {
				violations = append(violations, Violation{
					File: path, Line: lineNum, Kind: VDuplicateTarget, Target: name,
					Message: fmt.Sprintf("duplicate target (first seen at line %d)", prev.Line),
				})
				continue
			}
			help := ""
			if hm := ruleWithHelpRE.FindStringSubmatch(trim); hm != nil {
				help = hm[2]
			}
			targets[name] = &targetInfo{Line: lineNum, Help: help}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}

	// Post-pass: enforce name + help + phony for every target.
	names := make([]string, 0, len(targets))
	for n := range targets {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		info := targets[name]
		if !kebabLowerRE.MatchString(name) {
			violations = append(violations, Violation{
				File: path, Line: info.Line, Kind: VNameNotKebab, Target: name,
				Message: "target name must match [a-z][a-z0-9-]* (lower-kebab-case)",
			})
		}
		if info.Help == "" {
			violations = append(violations, Violation{
				File: path, Line: info.Line, Kind: VMissingHelp, Target: name,
				Message: "missing `## <one-line description>` comment on definition line",
			})
		}
		if !phony[name] {
			violations = append(violations, Violation{
				File: path, Line: info.Line, Kind: VPhonyMissing, Target: name,
				Message: "target not listed in any .PHONY: line (use single-line .PHONY per spec § 2.3)",
			})
		}
	}

	// Doc-block check (spec-0.8 D-7 binary criterion).
	violations = append(violations, checkDocBlock(path, docLines)...)

	return violations, nil
}

// checkDocBlock asserts D-7's 3 binary criteria over the top-of-file doc
// comments. Returns 0 or 1 violations (single rolled-up message).
func checkDocBlock(path string, docLines []string) []Violation {
	// Heuristic category keywords (Chinese + English forms).
	categoryKeywords := []string{
		"用户日常", "user daily",
		"CI", "CI 投影",
		"release",
		"维护", "maintenance",
		"开发", "dev",
	}
	categoriesSeen := map[string]bool{}
	hasCrossRepo := false
	hasMakeRequirement := false

	for _, line := range docLines {
		for _, kw := range categoryKeywords {
			if strings.Contains(line, kw) {
				categoriesSeen[kw] = true
			}
		}
		if strings.Contains(line, "../opendbx") || strings.Contains(line, "../opendbrb") {
			hasCrossRepo = true
		}
		l := strings.ToLower(line)
		if strings.Contains(l, "gnu make") || strings.Contains(l, "gmake") ||
			strings.Contains(l, "bash") || strings.Contains(l, "posix shell") {
			hasMakeRequirement = true
		}
	}

	var missing []string
	if len(categoriesSeen) < 3 {
		missing = append(missing, fmt.Sprintf("≥ 3 category headings (found %d in doc block)", len(categoriesSeen)))
	}
	if !hasCrossRepo {
		missing = append(missing, "≥ 1 cross-repo dependency line mentioning ../opendbx or ../opendbrb")
	}
	if !hasMakeRequirement {
		missing = append(missing, "≥ 1 line stating GNU make / bash / POSIX shell requirement")
	}
	if len(missing) == 0 {
		return nil
	}
	return []Violation{{
		File: path, Line: 0, Kind: VDocBlock,
		Message: "doc-block at top of Makefile missing: " + strings.Join(missing, "; "),
	}}
}

func main() {
	verbose := flag.Bool("v", false, "verbose: list parsed targets per file")
	flag.Parse()
	paths := flag.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "usage: makefile-check <Makefile> [Makefile2 ...]")
		os.Exit(3)
	}

	totalViolations := 0
	for _, p := range paths {
		violations, err := Check(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "makefile-check: %v\n", err)
			os.Exit(3)
		}
		if *verbose {
			fmt.Fprintf(os.Stderr, "makefile-check: %s scanned\n", p)
		}
		if len(violations) > 0 {
			fmt.Fprintf(os.Stderr, "makefile-check: %s — %d violation(s):\n", p, len(violations))
			for _, v := range violations {
				fmt.Fprintln(os.Stderr, v)
			}
			totalViolations += len(violations)
		}
	}

	if totalViolations > 0 {
		fmt.Fprintf(os.Stderr, "makefile-check FAIL: %d total violation(s) across %d file(s)\n",
			totalViolations, len(paths))
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "makefile-check OK (%d file(s) scanned)\n", len(paths))
}
