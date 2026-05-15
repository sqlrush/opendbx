// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// IMP-8 runewidth-wrap: forbid imports of `github.com/mattn/go-runewidth`
// outside the canonical wrapper `internal/app/cli/render/width`.
//
// Rationale (CLAUDE.md § 3.1):
//
// opendbx renders mixed-width content (ASCII + CJK + emoji) where
// East-Asian Ambiguous semantics differ between locales / runewidth
// versions. To guarantee consistent string-width math across the whole
// codebase, every caller MUST go through `render/width.Width()` rather
// than calling `runewidth.StringWidth(...)` directly. This rule rejects
// any import of go-runewidth from outside `internal/app/cli/render/width/...`.
//
// NB: import-rules-check sees import edges, not call sites. A wrapper
// function that re-exports runewidth would defeat this rule textually.
// Spec-0.11 may add an AST-level call-site checker as a complement.

package rules

import (
	"fmt"
	"strings"
)

// RunewidthModule is the forbidden import path; only the canonical
// wrapper package may import it.
const RunewidthModule = "github.com/mattn/go-runewidth"

// RunewidthAllowedPrefix is the canonical wrapper home (spec-0.10 R2
// codex LOW-6: prefix-safe match).
const RunewidthAllowedPrefix = ModulePrefix + "internal/app/cli/render/width"

// RunewidthTemporaryAllowed lists packages with TEMPORARY exemption to
// IMP-8. spec-0.11.5 D-1 + Q7 R2 拍板: uiinvariant directly imports
// runewidth until spec-1.14 render/width.Width() lands, then revert.
var RunewidthTemporaryAllowed = []string{
	ModulePrefix + "internal/testing/uiinvariant", // spec-0.11.5
}

// CheckRunewidthWrap returns "" if the from→to edge is allowed, or a
// violation if from imports go-runewidth outside the wrapper home.
func CheckRunewidthWrap(from, to string) string {
	if to != RunewidthModule && !strings.HasPrefix(to, RunewidthModule+"/") {
		return ""
	}
	if from == RunewidthAllowedPrefix || strings.HasPrefix(from, RunewidthAllowedPrefix+"/") {
		return ""
	}
	for _, p := range RunewidthTemporaryAllowed {
		if from == p || strings.HasPrefix(from, p+"/") {
			return ""
		}
	}
	return fmt.Sprintf(
		"IMP-8 runewidth-wrap: %q imports runewidth %q directly; only %q may; route through render/width.Width() (CLAUDE.md § 3.1)",
		from, to, RunewidthAllowedPrefix)
}
