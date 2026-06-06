// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File prompt.go — system-prompt "available skills" section (spec-2.3
// D-2). Pure, stdlib-only — this file keeps the app/skills leaf
// invariant (spec-2.2 D-9: no diagnose/config/logger/render imports; the
// invoke subpackage is likewise off-limits, spec-2.3 D-8 directional
// rule). Output format is provisional vs CC (survey § 7 T-11) and is
// errata-aligned after T-11 verify.

package skills

import (
	"sort"
	"strings"
)

// promptHeader precedes the per-skill list. The wording tells the model
// HOW to act on the list (invoke via the Skill tool) — the per-skill
// "when to use" signal lives in each description (CLAUDE.md § 3.3).
const promptHeader = "## Available skills\n\n" +
	"Invoke a skill with the Skill tool when its description matches the task.\n"

// PromptSection renders the system-prompt block advertising the active
// skill set (spec-2.3 D-2). Deterministic: skills are listed name-sorted
// and each description is reduced to its first non-empty line (CRLF
// normalized, TrimSpace; rule pinned by spec-2.3 R2). Returns "" for an
// empty input — the caller skips injection entirely (Q6).
//
// v1 discovery runs once at startup, so the returned bytes are stable
// for the whole session (prompt-cache friendly; spec-2.3 Q10 — note the
// system prompt transitions empty→non-empty when skills are present).
func PromptSection(active []Skill) string {
	if len(active) == 0 {
		return ""
	}
	names := make([]string, 0, len(active))
	byName := make(map[string]Skill, len(active))
	for _, sk := range active {
		key := sk.Key()
		if _, dup := byName[key]; dup {
			continue // defensive: Active guarantees unique keys (2.2);
			// first occurrence wins so a contract break upstream cannot
			// produce duplicate listing lines (post-impl cr NIT-1)
		}
		names = append(names, key)
		byName[key] = sk
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(promptHeader)
	for _, name := range names {
		b.WriteString("- ")
		b.WriteString(name)
		b.WriteString(": ")
		b.WriteString(firstNonEmptyLine(byName[name].Schema.Description))
		b.WriteString("\n")
	}
	return b.String()
}

// firstNonEmptyLine returns the first line of s whose TrimSpace is
// non-empty, itself TrimSpace'd. CRLF is normalized to LF before
// splitting so the pick is byte-stable across line-ending styles
// (spec-2.3 R2 pinned extraction rule). Returns "" when every line is
// blank — Validate guarantees a non-blank description for active skills,
// so that case is defensive only.
func firstNonEmptyLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}
