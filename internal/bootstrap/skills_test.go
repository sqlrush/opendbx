// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package bootstrap

import (
	"path/filepath"
	"strings"
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

// mkActiveSkill builds a minimal valid active Skill for skillsForChat tests
// (spec-2.3 D-5).
func mkActiveSkill(name string) skills.Skill {
	return skills.Skill{
		Schema: skills.Schema{Name: name, Description: "Test skill."},
		Body:   "body of " + name,
	}
}

// TestSkillsForChat_ZeroActive — no active skills → no executor, no
// system-prompt section (spec-2.3 Q6: the Skill wire surface is absent).
func TestSkillsForChat_ZeroActive(t *testing.T) {
	t.Parallel()
	execs, prompt := skillsForChat(skills.DiscoveryResult{})
	if execs != nil || prompt != "" {
		t.Errorf("skillsForChat(empty) = %v, %q; want nil, empty", execs, prompt)
	}
}

// TestSkillsForChat_Active — active skills yield exactly one executor
// (wire name "Skill") plus the prompt section listing each skill.
func TestSkillsForChat_Active(t *testing.T) {
	t.Parallel()
	res := skills.DiscoveryResult{Active: []skills.Skill{mkActiveSkill("alpha"), mkActiveSkill("beta")}}
	execs, prompt := skillsForChat(res)
	if len(execs) != 1 || execs[0].Name() != "Skill" {
		t.Fatalf("execs = %v; want one executor named Skill", execs)
	}
	for _, want := range []string{"## Available skills", "- alpha:", "- beta:"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt %q missing %q", prompt, want)
		}
	}
}

// TestSkillsForChat_ContractViolation_LogSkipNoPanic — a duplicate Key in
// Active (upstream spec-2.2 contract violation) must be skipped, never
// panic: interact continues without skills (user decision 4/4 2026-06-05).
func TestSkillsForChat_ContractViolation_LogSkipNoPanic(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("skillsForChat panicked on contract violation: %v", r)
		}
	}()
	res := skills.DiscoveryResult{Active: []skills.Skill{mkActiveSkill("dup"), mkActiveSkill("dup")}}
	execs, prompt := skillsForChat(res)
	if execs != nil || prompt != "" {
		t.Errorf("contract violation must skip skills entirely, got %v, %q", execs, prompt)
	}
}

// TestDiagnoseRegistryWith_SkillTool — the registry composes clock + echo
// + the extra executor; the no-extra form matches the legacy default.
func TestDiagnoseRegistryWith_SkillTool(t *testing.T) {
	t.Parallel()
	execs, _ := skillsForChat(skills.DiscoveryResult{Active: []skills.Skill{mkActiveSkill("alpha")}})
	reg := diagnoseRegistryWith(execs...)
	for _, want := range []string{"Skill", "clock", "echo"} {
		if _, ok := reg.Get(want); !ok {
			t.Errorf("registry missing %q", want)
		}
	}
	if names := defaultDiagnoseRegistry().Names(); len(names) != 2 {
		t.Errorf("default registry = %v; want clock+echo only", names)
	}
}
