// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package skills

import (
	"errors"
	"strings"
	"testing"
)

func skillWith(name, desc string) Skill {
	return Skill{Schema: Schema{Name: name, Description: desc}}
}

func TestValidate_OK(t *testing.T) {
	t.Parallel()
	rep, err := Validate(skillWith("code-reviewer", "reviews code"))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("unexpected warnings: %+v", rep.Warnings)
	}
}

func TestValidate_MissingName(t *testing.T) {
	t.Parallel()
	_, err := Validate(skillWith("   ", "desc"))
	assertWraps(t, err, ErrMissingName.Code())
}

func TestValidate_MissingDescription(t *testing.T) {
	t.Parallel()
	// whitespace-only description is treated as missing (TrimSpace).
	_, err := Validate(skillWith("ok-name", "   \n  "))
	assertWraps(t, err, ErrMissingDescription.Code())
}

// TestValidate_LooseNameMatrix — Q5 loose rule: path/control chars reject;
// non-kebab (but legal) names pass with a warning.
func TestValidate_LooseNameMatrix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		skillID  string
		wantErr  string // "" = no error
		wantWarn bool   // WarnNonKebabName expected
	}{
		{"kebab ok", "code-reviewer", "", false},
		{"single word", "explain", "", false},
		{"uppercase warns", "CodeReviewer", "", true},
		{"underscore warns", "code_reviewer", "", true},
		{"pure numeric warns", "123", "", true},
		{"slash rejected", "a/b", ErrInvalidName.Code(), false},
		{"backslash rejected", "a\\b", ErrInvalidName.Code(), false},
		{"space rejected", "a b", ErrInvalidName.Code(), false},
		{"too long rejected", strings.Repeat("a", maxNameLen+1), ErrInvalidName.Code(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rep, err := Validate(skillWith(tc.skillID, "desc"))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want no error, got %v", err)
				}
			} else {
				assertWraps(t, err, tc.wantErr)
			}
			hasWarn := false
			for _, w := range rep.Warnings {
				if w.Kind == WarnNonKebabName {
					hasWarn = true
				}
			}
			if hasWarn != tc.wantWarn {
				t.Errorf("kebab warning = %v; want %v (warnings: %+v)", hasWarn, tc.wantWarn, rep.Warnings)
			}
		})
	}
}

// TestValidate_MultiError — multiple failures aggregate under
// SKILL.VALIDATION_FAILED while errors.Is still matches each detail (Q7).
func TestValidate_MultiError(t *testing.T) {
	t.Parallel()
	_, err := Validate(skillWith("", "")) // missing name AND description
	if err == nil {
		t.Fatal("expected error")
	}
	assertWraps(t, err, ErrValidationFailed.Code())
	if !errors.Is(err, ErrMissingName) {
		t.Error("errors.Is should match ErrMissingName under the aggregate")
	}
	if !errors.Is(err, ErrMissingDescription) {
		t.Error("errors.Is should match ErrMissingDescription under the aggregate")
	}
}

// TestValidate_UnknownFieldWarns — preserved Extra keys surface as sorted
// warnings, not failures (Q4).
func TestValidate_UnknownFieldWarns(t *testing.T) {
	t.Parallel()
	s := Skill{Schema: Schema{
		Name:        "ok",
		Description: "d",
		Extra:       map[string]any{"zeta": 1, "alpha": 2},
	}}
	rep, err := Validate(s)
	if err != nil {
		t.Fatalf("unknown fields must not fail validation: %v", err)
	}
	var unknown []string
	for _, w := range rep.Warnings {
		if w.Kind == WarnUnknownField {
			unknown = append(unknown, w.Field)
		}
	}
	if len(unknown) != 2 || unknown[0] != "alpha" || unknown[1] != "zeta" {
		t.Errorf("unknown-field warnings = %v; want sorted [alpha zeta]", unknown)
	}
}

func assertWraps(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error wrapping %s, got nil", code)
	}
	// The detail code must be reachable via errors.Is on the registered sentinel.
	found := false
	for _, sent := range allSkillSentinels() {
		if sent.Code() == code && errors.Is(err, sent) {
			found = true
		}
	}
	if !found {
		t.Errorf("error does not wrap code %s: %v", code, err)
	}
}
