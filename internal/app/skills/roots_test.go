// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package skills

import "testing"

// TestAssignPrecedence_Injective — every root gets a distinct precedence
// regardless of plugin/search-path count (review HIGH-3: no cross-root ties).
func TestAssignPrecedence_Injective(t *testing.T) {
	t.Parallel()
	specs := []RootSpec{
		{Kind: SourceBuiltin, Band: BandBuiltin, SubOrder: 0, Dir: "/builtin"},
		{Kind: SourcePluginCache, Band: BandPlugin, SubOrder: 0, Dir: "/p/a/skills", PluginID: "a"},
		{Kind: SourcePluginCache, Band: BandPlugin, SubOrder: 1, Dir: "/p/b/skills", PluginID: "b"},
		{Kind: SourceUserGlobal, Band: BandUser, SubOrder: VariantClaude, Dir: "/h/.claude/skills"},
		{Kind: SourceUserGlobal, Band: BandUser, SubOrder: VariantOpendbx, Dir: "/h/.opendbx/skills"},
		{Kind: SourceProject, Band: BandProject, SubOrder: VariantClaude, Dir: "/c/.claude/skills"},
		{Kind: SourceProject, Band: BandProject, SubOrder: VariantOpendbx, Dir: "/c/.opendbx/skills"},
		{Kind: SourceProject, Band: BandSearchPath, SubOrder: 0, Dir: "/custom1", Required: true},
		{Kind: SourceProject, Band: BandSearchPath, SubOrder: 1, Dir: "/custom2", Required: true},
	}
	roots := AssignPrecedence(specs)
	if len(roots) != len(specs) {
		t.Fatalf("len = %d; want %d", len(roots), len(specs))
	}
	seen := map[int]string{}
	for _, r := range roots {
		if prev, dup := seen[r.Precedence]; dup {
			t.Errorf("precedence %d tied: %s and %s", r.Precedence, prev, r.Dir)
		}
		seen[r.Precedence] = r.Dir
	}
}

// TestAssignPrecedence_Order — bands rank low→high; .opendbx outranks .claude;
// search paths outrank everything (Q3 + Q8 ladder).
func TestAssignPrecedence_Order(t *testing.T) {
	t.Parallel()
	specs := []RootSpec{
		{Band: BandSearchPath, SubOrder: 0, Dir: "/search"},
		{Band: BandBuiltin, SubOrder: 0, Dir: "/builtin"},
		{Band: BandProject, SubOrder: VariantOpendbx, Dir: "/proj/.opendbx"},
		{Band: BandProject, SubOrder: VariantClaude, Dir: "/proj/.claude"},
		{Band: BandUser, SubOrder: VariantClaude, Dir: "/user/.claude"},
	}
	roots := AssignPrecedence(specs)
	prec := map[string]int{}
	for _, r := range roots {
		prec[r.Dir] = r.Precedence
	}
	// Each dir must outrank the previous one (strictly ascending ladder).
	ladder := []string{"/builtin", "/user/.claude", "/proj/.claude", "/proj/.opendbx", "/search"}
	for i := 1; i < len(ladder); i++ {
		if prec[ladder[i-1]] >= prec[ladder[i]] {
			t.Errorf("ladder order wrong: %s(%d) should be < %s(%d)",
				ladder[i-1], prec[ladder[i-1]], ladder[i], prec[ladder[i]])
		}
	}
}

// TestAssignPrecedence_DedupsDir — the same dir under two specs collapses to
// one root (highest-ranked), so a dir cannot shadow itself (review codex).
func TestAssignPrecedence_DedupsDir(t *testing.T) {
	t.Parallel()
	specs := []RootSpec{
		{Band: BandUser, SubOrder: 0, Dir: "/shared/skills"},
		{Band: BandSearchPath, SubOrder: 0, Dir: "/shared/skills", Required: true}, // same dir, higher band
		{Band: BandProject, SubOrder: 0, Dir: "/proj/skills"},
	}
	roots := AssignPrecedence(specs)
	if len(roots) != 2 {
		t.Fatalf("duplicate dir should collapse to 1 root: %d roots %+v", len(roots), roots)
	}
	for _, r := range roots {
		if r.Dir == "/shared/skills" && !r.Required {
			t.Errorf("dedup should keep the highest-ranked (Required) occurrence: %+v", r)
		}
	}
}

// TestAssignPrecedence_Deterministic — input order does not change the result.
func TestAssignPrecedence_Deterministic(t *testing.T) {
	t.Parallel()
	a := []RootSpec{
		{Band: BandUser, SubOrder: 0, Dir: "/b"},
		{Band: BandUser, SubOrder: 0, Dir: "/a"},
	}
	b := []RootSpec{
		{Band: BandUser, SubOrder: 0, Dir: "/a"},
		{Band: BandUser, SubOrder: 0, Dir: "/b"},
	}
	ra, rb := AssignPrecedence(a), AssignPrecedence(b)
	for i := range ra {
		if ra[i].Dir != rb[i].Dir || ra[i].Precedence != rb[i].Precedence {
			t.Errorf("non-deterministic: %v vs %v", ra, rb)
		}
	}
}
