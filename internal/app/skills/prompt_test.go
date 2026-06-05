// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package skills

import (
	"strings"
	"testing"
)

// mkSkill builds a minimal valid Skill for prompt tests.
func mkSkill(name, desc string) Skill {
	return Skill{Schema: Schema{Name: name, Description: desc}}
}

// TestPromptSection_Empty — empty active set renders nothing (the caller
// skips injection entirely; spec-2.3 Q6).
func TestPromptSection_Empty(t *testing.T) {
	t.Parallel()
	if got := PromptSection(nil); got != "" {
		t.Errorf("PromptSection(nil) = %q, want empty", got)
	}
	if got := PromptSection([]Skill{}); got != "" {
		t.Errorf("PromptSection(empty) = %q, want empty", got)
	}
}

// TestPromptSection_SortedDeterministic — listing is name-sorted
// regardless of input order, and byte-stable across calls (spec-2.3 D-2).
func TestPromptSection_SortedDeterministic(t *testing.T) {
	t.Parallel()
	a := []Skill{mkSkill("zeta", "Z skill."), mkSkill("alpha", "A skill.")}
	b := []Skill{mkSkill("alpha", "A skill."), mkSkill("zeta", "Z skill.")}
	ga, gb := PromptSection(a), PromptSection(b)
	if ga != gb {
		t.Fatalf("PromptSection not order-independent:\n%q\nvs\n%q", ga, gb)
	}
	ia, iz := strings.Index(ga, "- alpha:"), strings.Index(ga, "- zeta:")
	if ia < 0 || iz < 0 || ia > iz {
		t.Errorf("expected name-sorted listing, got:\n%s", ga)
	}
	if !strings.Contains(ga, "## Available skills") || !strings.Contains(ga, "Skill tool") {
		t.Errorf("missing header/instruction:\n%s", ga)
	}
}

// TestPromptSection_FirstLineExtraction — description reduces to the
// first non-empty line; CRLF normalized; surrounding space trimmed
// (spec-2.3 R2 pinned rule).
func TestPromptSection_FirstLineExtraction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		desc string
		want string
	}{
		{"single line", "Reviews code.", "Reviews code."},
		{"multi line", "Reviews code.\nSecond line ignored.", "Reviews code."},
		{"crlf", "Reviews code.\r\nIgnored.", "Reviews code."},
		{"leading blank lines", "\n  \nActual line.\nIgnored.", "Actual line."},
		{"surrounding spaces", "  padded line  \nIgnored.", "padded line"},
		{"block scalar trailing newline", "Expert reviewer.\n", "Expert reviewer."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := PromptSection([]Skill{mkSkill("s", tc.desc)})
			want := "- s: " + tc.want + "\n"
			if !strings.Contains(got, want) {
				t.Errorf("desc %q: section %q missing line %q", tc.desc, got, want)
			}
		})
	}
}

// TestFirstNonEmptyLine_AllBlank — defensive: all-blank description
// yields "" (Validate prevents this for active skills).
func TestFirstNonEmptyLine_AllBlank(t *testing.T) {
	t.Parallel()
	if got := firstNonEmptyLine(" \n\t\n"); got != "" {
		t.Errorf("firstNonEmptyLine(all blank) = %q, want empty", got)
	}
}
