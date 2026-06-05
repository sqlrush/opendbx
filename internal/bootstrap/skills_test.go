// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package bootstrap

import (
	"path/filepath"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/skills"
	"github.com/sqlrush/opendbx/internal/platform/config"
)

func dirByName(roots []skills.SkillRoot) map[string]skills.SkillRoot {
	m := make(map[string]skills.SkillRoot, len(roots))
	for _, r := range roots {
		m[r.Dir] = r
	}
	return m
}

// TestBuildSkillRoots_DerivesFromHomeAndCWD — roots come from HomeDir + CWD
// directly, NOT from config SourcePaths (review HIGH-1).
func TestBuildSkillRoots_DerivesFromHomeAndCWD(t *testing.T) {
	t.Parallel()
	in := SkillPathInputs{HomeDir: "/home/u", CWD: "/proj"}
	opts := BuildSkillRoots(in, config.PluginsConfig{})
	dirs := dirByName(opts.Roots)
	for _, want := range []string{
		"/home/u/.claude/skills", "/home/u/.opendbx/skills",
		"/proj/.claude/skills", "/proj/.opendbx/skills",
	} {
		if _, ok := dirs[want]; !ok {
			t.Errorf("missing derived root %q (have %v)", want, keys(dirs))
		}
	}
}

// TestBuildSkillRoots_Ladder — project outranks user; .opendbx outranks
// .claude; search paths outrank everything (Q3 + Q8).
func TestBuildSkillRoots_Ladder(t *testing.T) {
	t.Parallel()
	in := SkillPathInputs{HomeDir: "/h", CWD: "/c"}
	cfg := config.PluginsConfig{SkillSearchPaths: []string{"/custom/skills"}}
	opts := BuildSkillRoots(in, cfg)
	p := map[string]int{}
	for _, r := range opts.Roots {
		p[r.Dir] = r.Precedence
	}
	checks := [][2]string{
		{"/h/.claude/skills", "/h/.opendbx/skills"},
		{"/h/.opendbx/skills", "/c/.claude/skills"},
		{"/c/.claude/skills", "/c/.opendbx/skills"},
		{"/c/.opendbx/skills", "/custom/skills"},
	}
	for _, c := range checks {
		if p[c[0]] >= p[c[1]] {
			t.Errorf("precedence %s(%d) should be < %s(%d)", c[0], p[c[0]], c[1], p[c[1]])
		}
	}
}

// TestBuildSkillRoots_SearchPathRequiredAndCleaned — config search paths are
// Required and resolved against cwd.
func TestBuildSkillRoots_SearchPathRequiredAndCleaned(t *testing.T) {
	t.Parallel()
	in := SkillPathInputs{HomeDir: "/h", CWD: "/c"}
	cfg := config.PluginsConfig{SkillSearchPaths: []string{"rel/skills", "/abs/skills"}}
	opts := BuildSkillRoots(in, cfg)
	dirs := dirByName(opts.Roots)
	rel, ok := dirs[filepath.Join("/c", "rel", "skills")]
	if !ok || !rel.Required {
		t.Errorf("relative search path should resolve under cwd + be Required: %v", keys(dirs))
	}
	if abs, ok := dirs["/abs/skills"]; !ok || !abs.Required {
		t.Errorf("absolute search path should be kept + Required")
	}
}

// TestBuildSkillRoots_Plugins — installed plugin dirs become injective roots.
func TestBuildSkillRoots_Plugins(t *testing.T) {
	t.Parallel()
	in := SkillPathInputs{
		HomeDir: "/h", CWD: "/c",
		PluginDirs: []PluginDir{{ID: "a", Dir: "/cache/a/skills"}, {ID: "b", Dir: "/cache/b/skills"}},
	}
	opts := BuildSkillRoots(in, config.PluginsConfig{})
	seen := map[int]bool{}
	for _, r := range opts.Roots {
		if seen[r.Precedence] {
			t.Errorf("precedence %d not injective", r.Precedence)
		}
		seen[r.Precedence] = true
	}
	dirs := dirByName(opts.Roots)
	if dirs["/cache/a/skills"].PluginID != "a" {
		t.Errorf("plugin id not carried: %+v", dirs["/cache/a/skills"])
	}
}

// TestBuildSkillRoots_PluginsSortedByID — cross-plugin precedence is by sorted
// PluginID regardless of input order (locked Q8; codex PREC-HIGH-01).
func TestBuildSkillRoots_PluginsSortedByID(t *testing.T) {
	t.Parallel()
	base := SkillPathInputs{HomeDir: "/h", CWD: "/c"}
	forward := base
	forward.PluginDirs = []PluginDir{{ID: "alpha", Dir: "/p/alpha"}, {ID: "beta", Dir: "/p/beta"}}
	reverse := base
	reverse.PluginDirs = []PluginDir{{ID: "beta", Dir: "/p/beta"}, {ID: "alpha", Dir: "/p/alpha"}}

	precOf := func(in SkillPathInputs, dir string) int {
		for _, r := range BuildSkillRoots(in, config.PluginsConfig{}).Roots {
			if r.Dir == dir {
				return r.Precedence
			}
		}
		return -1
	}
	// alpha < beta by sorted ID, in BOTH input orders.
	if precOf(forward, "/p/alpha") >= precOf(forward, "/p/beta") {
		t.Error("alpha should outrank-less beta (forward order)")
	}
	if precOf(reverse, "/p/alpha") >= precOf(reverse, "/p/beta") {
		t.Error("plugin precedence flipped with input order (not sorted by ID)")
	}
}

func TestBuildSkillRoots_DisabledPassthrough(t *testing.T) {
	t.Parallel()
	opts := BuildSkillRoots(SkillPathInputs{HomeDir: "/h", CWD: "/c"},
		config.PluginsConfig{DisabledSkills: []string{"x", "y"}})
	if len(opts.Disabled) != 2 {
		t.Errorf("disabled list not passed through: %v", opts.Disabled)
	}
}

func keys(m map[string]skills.SkillRoot) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
