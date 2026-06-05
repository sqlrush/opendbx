// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File skill.go — Skill tool adapter (spec-2.3 D-4).
//
// CC shows a "Using <skill-name>" banner when the model enters a skill
// (survey skills.md § 2.3); the exact CC tool-row text is line-level TBD
// (survey § 7 T-11), so this header is the spec-2.3 pinned provisional
// form: "Skill(<name>)" plus a compact args summary when args are
// present. Errata-aligned after T-11 verify (spec-2.1 R-1 precedent).

package adapter

import "strings"

// Skill implements HeaderRenderer for the spec-2.3 SkillTool
// (wire name "Skill"; input {skill, args}).
type Skill struct{}

// RenderHeader returns `Skill(<name>)`, appending a sorted compact
// `k=v` args summary when a non-empty args object is present. A missing
// or non-string skill field renders the "(no skill)" placeholder
// (mirrors Bash's "(no command)" convention).
func (Skill) RenderHeader(input map[string]any, ctx Context) (string, error) {
	name, ok := input["skill"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return "Skill(no skill)", nil
	}
	head := "Skill(" + name + ")"
	args, ok := input["args"].(map[string]any)
	if !ok || len(args) == 0 {
		return head, nil
	}
	maxLen := ctx.Cols / 2
	if maxLen < 8 {
		maxLen = 8
	}
	summary := compactArgs(args, maxLen-len(head)-1)
	if summary == "" {
		return head, nil
	}
	return head + " " + summary, nil
}

func init() {
	Default.Register("Skill", Skill{})
}
