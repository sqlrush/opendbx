// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File scope.go — allowed-tools scope filtering (spec-2.3 D-3).
//
// A successful ToolOutput carrying a non-nil ToolFilter REPLACES the
// run-local execution-tool scope: subsequent turns advertise only the
// filtered set, and a dispatch of a registered-but-filtered tool is
// answered with a recoverable SKILL.SCOPE_TOOL_DENIED tool_result (never
// terminal — the model self-corrects). Scope state lives entirely in
// Loop.Run locals; it dies with the Run (spec-2.3 R-6).
//
// "Skill" is implicitly retained in both the advertised set and the
// dispatch guard (spec-2.3 Q13, user decision 4/4): it is the
// scope-control verb — keeping it reachable means a Run can always
// switch scope, while execution tools stay filtered. This is a
// scope-control exception, NOT a permission bypass.

package diagnose

import "github.com/sqlrush/opendbx/internal/domain/llm"

// skillToolName is the wire name of the scope-control tool (spec-2.3
// Q3 CC parity). Kept as a local constant because diagnose cannot import
// app/skills/invoke (invoke imports diagnose); the integration suite
// asserts invoke.ToolName == this value to prevent drift.
const skillToolName = "Skill"

// applyFilter narrows the advertised tool set to the active scope.
// nil filter → tools returned unchanged (same slice identity, keeping
// the no-skill path byte-stable for prompt caching). A non-nil filter
// returns a NEW slice (Rule 12 immutability — the shared backing array is never
// mutated) holding tools whose Name is in the filter, plus "Skill".
func applyFilter(tools []llm.ToolSchema, filter []string) []llm.ToolSchema {
	if filter == nil {
		return tools
	}
	allowed := make(map[string]bool, len(filter)+1)
	for _, n := range filter {
		allowed[n] = true
	}
	allowed[skillToolName] = true
	out := make([]llm.ToolSchema, 0, len(tools))
	for _, ts := range tools {
		if allowed[ts.Name] {
			out = append(out, ts)
		}
	}
	return out
}

// normalizeFilter canonicalizes a ToolFilter before it becomes the
// active scope (spec-2.3 R2 pinned contract): case-sensitive dedupe
// preserving first-occurrence order; nil stays nil (inherit — no scope
// change); a non-nil empty slice stays non-nil (an empty execution
// scope: only "Skill" remains callable). Entries are already trimmed
// upstream (Schema.AllowedToolsList filters blanks).
func normalizeFilter(filter []string) []string {
	if filter == nil {
		return nil
	}
	seen := make(map[string]bool, len(filter))
	out := make([]string, 0, len(filter))
	for _, n := range filter {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// scopeDenied reports whether the active filter blocks a dispatch of
// name. nil filter denies nothing; "Skill" is never denied (Q13).
func scopeDenied(filter []string, name string) bool {
	if filter == nil || name == skillToolName {
		return false
	}
	for _, n := range filter {
		if n == name {
			return false
		}
	}
	return true
}
