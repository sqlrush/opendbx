// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File validate.go — Skill validation (spec-2.1 D-4). Pure, no I/O.
//
// Warnings are returned as DATA (ValidationReport), never logged here — that
// keeps app/skills a pure leaf (no logger import; review CRIT). 2.2/UI decide
// how to surface warnings.
//
// Hard failures aggregate (Q7 multi-error): each detail is an errcode
// sentinel; they are joined and wrapped under SKILL.VALIDATION_FAILED so the
// sidecar sees one primary code while errors.Is still matches each detail.

package skills

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

const maxNameLen = 128

// kebabNameRE is the CC-convention shape (lowercase-with-hyphens). Non-match
// is a WARNING, not a failure (Q5 loose: CC has not pinned a name regex, so
// rejecting non-kebab could refuse a valid CC skill).
var kebabNameRE = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// WarnKind classifies a non-fatal validation warning.
type WarnKind int

const (
	// WarnNonKebabName — name is valid but not kebab-case.
	WarnNonKebabName WarnKind = iota
	// WarnUnknownField — a frontmatter key was preserved in Extra.
	WarnUnknownField
)

// Warning is a non-fatal validation note returned as data.
type Warning struct {
	Kind   WarnKind
	Field  string
	Detail string
}

// ValidationReport carries the warnings from a Validate call. err (returned
// separately) carries the hard failures.
type ValidationReport struct {
	Warnings []Warning
}

// Validate checks a parsed Skill. It returns warnings as data and, when one
// or more hard rules fail, a SKILL.VALIDATION_FAILED error wrapping the joined
// detail sentinels. A skill with no failures returns (report, nil).
func Validate(s Skill) (ValidationReport, error) {
	var report ValidationReport
	var failures []error

	name := s.Schema.Name
	switch {
	case strings.TrimSpace(name) == "":
		failures = append(failures, errcode.New(ErrMissingName.Code(), "", ""))
	case !validName(name):
		failures = append(failures, errcode.Newf(ErrInvalidName.Code(),
			"name %q has illegal characters or exceeds %d chars", name, maxNameLen))
	default:
		if !kebabNameRE.MatchString(name) {
			report.Warnings = append(report.Warnings, Warning{
				Kind:   WarnNonKebabName,
				Field:  "name",
				Detail: "name is not kebab-case (lowercase-with-hyphens); CC convention",
			})
		}
	}

	if strings.TrimSpace(s.Schema.Description) == "" {
		failures = append(failures, errcode.New(ErrMissingDescription.Code(), "", ""))
	}

	// Unknown-field warnings (sorted for determinism). Preserved, not
	// rejected (Q4 forward-compat); not silently dropped (规则 7).
	if len(s.Schema.Extra) > 0 {
		keys := make([]string, 0, len(s.Schema.Extra))
		for k := range s.Schema.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			report.Warnings = append(report.Warnings, Warning{
				Kind:   WarnUnknownField,
				Field:  k,
				Detail: "unknown frontmatter field preserved in Extra (forward-compat)",
			})
		}
	}

	if len(failures) == 0 {
		return report, nil
	}
	// Single or multiple: always wrap under the aggregate primary code so the
	// sidecar reads a stable SKILL.VALIDATION_FAILED while details survive
	// under Unwrap / errors.Is (Q7).
	return report, errcode.Wrap(ErrValidationFailed.Code(), errors.Join(failures...), "", "")
}

// validName applies the loose name rule (Q5): no path separators, whitespace,
// or control characters, and within the length cap. kebab-case is only a
// warning, checked separately.
func validName(name string) bool {
	if len(name) > maxNameLen {
		return false
	}
	for _, r := range name {
		if r == '/' || r == '\\' || unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}
