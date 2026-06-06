// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package invoke

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// TestErrorsRegistered — both invoke-side sentinels carry the Rule 7
// three-piece contract and the SKILL. prefix (spec-2.3 D-6).
func TestErrorsRegistered(t *testing.T) {
	t.Parallel()
	for _, s := range []errcode.Sentinel{ErrNotFound, ErrInvokeInvalid} {
		if !strings.HasPrefix(s.Code(), "SKILL.") {
			t.Errorf("code %q lacks SKILL. prefix", s.Code())
		}
		if s.Message() == "" || s.Hint() == "" {
			t.Errorf("%s missing message/hint (Rule 7 triple)", s.Code())
		}
	}
}

// TestNotFoundContent — template carries code, requested name, the sorted
// active list, and an actionable hint (spec-2.3 D-6 template).
func TestNotFoundContent(t *testing.T) {
	t.Parallel()
	got := notFoundContent("nope", []string{"alpha", "beta"})
	for _, want := range []string{
		"SKILL.NOT_FOUND",
		`"nope"`,
		"Available skills: [alpha, beta]",
		"Hint: check /debug skills",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("notFoundContent missing %q in %q", want, got)
		}
	}
}

// TestInvokeInvalidContent — template carries code, the detail variant,
// and the call-shape hint (spec-2.3 D-6 template).
func TestInvokeInvalidContent(t *testing.T) {
	t.Parallel()
	got := invokeInvalidContent(`missing or non-string "skill" field`)
	for _, want := range []string{
		"SKILL.INVOKE_INVALID",
		`missing or non-string "skill" field`,
		`{"skill": "<name>"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("invokeInvalidContent missing %q in %q", want, got)
		}
	}
}
