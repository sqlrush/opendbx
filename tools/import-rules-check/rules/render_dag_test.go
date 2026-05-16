// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package rules

import (
	"strings"
	"testing"
)

// renderPath constructs the import path for a render subpackage by name.
func renderPath(name string) string {
	return RenderRoot + name
}

// TestRenderDAG_FullMatrix verifies every pair (from, to) in the 10-package
// RenderOrder against the post-spec-0.13 rule (idx_from > idx_to allowed,
// idx_from <= idx_to forbidden). 10×10 = 100 cases.
func TestRenderDAG_FullMatrix(t *testing.T) {
	t.Parallel()
	for fi, from := range RenderOrder {
		for ti, to := range RenderOrder {
			fi, ti := fi, ti
			from, to := from, to
			name := from + "_imports_" + to
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				got := CheckRenderDAG(renderPath(from), renderPath(to))
				shouldFail := fi <= ti
				if shouldFail && got == "" {
					t.Errorf("expected violation for %s(%d) → %s(%d); got OK", from, fi, to, ti)
				}
				if !shouldFail && got != "" {
					t.Errorf("expected OK for %s(%d) → %s(%d); got violation: %s", from, fi, to, ti, got)
				}
			})
		}
	}
}

// TestRenderDAG_BREAKING_RegressionCases lists the 6 critical edges
// from spec-0.13 § 2.2 (R2 CRIT-1 sequence reordering + R2 H-1 operator
// flip). These are the edges that the pre-spec-0.13 IMP-6 ruled
// differently — they MUST land in the new direction or the spec didn't
// land correctly.
func TestRenderDAG_BREAKING_RegressionCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		from, to  string
		wantAllow bool
	}{
		// block(7) → layout(4): root reaches leaf → OK
		{"block_imports_layout_OK", "block", "layout", true},
		// layout(4) → block(7): leaf cannot reach root → FAIL
		{"layout_imports_block_FAIL", "layout", "block", false},
		// scrollback(8) → block(7): higher root reaches block → OK
		{"scrollback_imports_block_OK", "scrollback", "block", true},
		// block(7) → scrollback(8): block cannot reach scrollback → FAIL
		{"block_imports_scrollback_FAIL", "block", "scrollback", false},
		// streaming(9) → scrollback(8): true root reaches scrollback → OK
		{"streaming_imports_scrollback_OK", "streaming", "scrollback", true},
		// scrollback(8) → streaming(9): scrollback cannot reach streaming → FAIL
		{"scrollback_imports_streaming_FAIL", "scrollback", "streaming", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := CheckRenderDAG(renderPath(c.from), renderPath(c.to))
			if c.wantAllow && got != "" {
				t.Errorf("expected OK; got violation: %s", got)
			}
			if !c.wantAllow && got == "" {
				t.Errorf("expected violation; got OK")
			}
			if !c.wantAllow && got != "" && !strings.Contains(got, "leaf→root") {
				t.Errorf("violation message should mention 'leaf→root'; got: %s", got)
			}
		})
	}
}

// TestRenderDAG_UnknownSubpackage verifies that adding a new render/*
// directory without updating RenderOrder produces a clear error.
func TestRenderDAG_UnknownSubpackage(t *testing.T) {
	t.Parallel()
	got := CheckRenderDAG(RenderRoot+"unknownpkg", renderPath("width"))
	if !strings.Contains(got, "not in RenderOrder") {
		t.Errorf("expected 'not in RenderOrder' error; got: %s", got)
	}
	got2 := CheckRenderDAG(renderPath("width"), RenderRoot+"unknownpkg")
	if !strings.Contains(got2, "not in RenderOrder") {
		t.Errorf("expected 'not in RenderOrder' error for target; got: %s", got2)
	}
}

// TestRenderDAG_NonRenderImports verifies that edges where neither
// endpoint is under render/ are ignored.
func TestRenderDAG_NonRenderImports(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		from, to string
	}{
		{"both_stdlib", "fmt", "errors"},
		{"both_internal_non_render", ModulePrefix + "internal/platform/errcode", ModulePrefix + "internal/platform/version"},
		{"one_render_one_outside", renderPath("width"), ModulePrefix + "internal/platform/errcode"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := CheckRenderDAG(c.from, c.to); got != "" {
				t.Errorf("expected OK (non-render path); got violation: %s", got)
			}
		})
	}
}

// TestRenderOrder_Sequence verifies the canonical leaf→root sequence
// is exactly what spec-0.13 § 2.2 documents (guard against accidental
// reordering).
func TestRenderOrder_Sequence(t *testing.T) {
	t.Parallel()
	want := []string{
		"width", "style", "terminal", "buffer", "layout",
		"optimizer", "scheduler", "block", "scrollback", "streaming",
	}
	if len(RenderOrder) != len(want) {
		t.Fatalf("RenderOrder length %d, want %d", len(RenderOrder), len(want))
	}
	for i, name := range want {
		if RenderOrder[i] != name {
			t.Errorf("RenderOrder[%d] = %q, want %q", i, RenderOrder[i], name)
		}
	}
}
