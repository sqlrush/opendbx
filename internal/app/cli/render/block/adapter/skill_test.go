// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package adapter

import (
	"strings"
	"testing"
)

// TestSkill_Registered — the Skill adapter self-registers under the
// CC-parity wire name (spec-2.3 D-4).
func TestSkill_Registered(t *testing.T) {
	t.Parallel()
	if Default.Lookup("Skill") == nil {
		t.Fatal("Skill adapter not registered in Default registry")
	}
}

// TestSkill_RenderHeader — table over name/args shapes (spec-2.3 D-4).
func TestSkill_RenderHeader(t *testing.T) {
	t.Parallel()
	ctx := Context{Cols: 80}
	tests := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{"plain", map[string]any{"skill": "code-reviewer"}, "Skill(code-reviewer)"},
		{"missing skill", map[string]any{}, "Skill(no skill)"},
		{"non-string skill", map[string]any{"skill": 7}, "Skill(no skill)"},
		{"blank skill", map[string]any{"skill": " "}, "Skill(no skill)"},
		{"empty args object", map[string]any{"skill": "s", "args": map[string]any{}}, "Skill(s)"},
		{"non-object args ignored for display", map[string]any{"skill": "s", "args": "x"}, "Skill(s)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Skill{}.RenderHeader(tc.input, ctx)
			if err != nil || got != tc.want {
				t.Errorf("RenderHeader(%v) = %q, %v; want %q", tc.input, got, err, tc.want)
			}
		})
	}
}

// TestSkill_RenderHeader_ArgsSummary — args render as a sorted compact
// summary after the head; narrow columns degrade to head-only.
func TestSkill_RenderHeader_ArgsSummary(t *testing.T) {
	t.Parallel()
	input := map[string]any{
		"skill": "topsql",
		"args":  map[string]any{"limit": "10", "db": "main"},
	}
	got, err := Skill{}.RenderHeader(input, Context{Cols: 120})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "Skill(topsql) ") {
		t.Errorf("want head prefix, got %q", got)
	}
	// Sorted keys: db before limit.
	if !strings.Contains(got, "db=") || strings.Index(got, "db=") > strings.Index(got, "limit=") {
		t.Errorf("args summary must be key-sorted: %q", got)
	}
	// Narrow terminal → summary dropped, head kept.
	gotNarrow, _ := Skill{}.RenderHeader(input, Context{Cols: 20})
	if gotNarrow != "Skill(topsql)" {
		t.Errorf("narrow cols should keep head only, got %q", gotNarrow)
	}
}
