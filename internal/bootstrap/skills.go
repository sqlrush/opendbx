// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File skills.go — wires config + paths into skill discovery options
// (spec-2.2 D-6). BuildSkillRoots is a PURE function of its inputs (no fs I/O —
// the scanning happens later in skills.Discover), so the home/cwd/plugin
// derivation is unit-testable without a real filesystem.
//
// Skill roots are derived DIRECTLY from HomeDir + CWD (review HIGH): config
// SourcePaths point at config FILES (and on macOS live under Application
// Support), which are the wrong basis for ~/.claude/skills-style roots.

package bootstrap

import (
	"os"
	"path/filepath"

	"github.com/sqlrush/opendbx/internal/app/skills"
	"github.com/sqlrush/opendbx/internal/platform/config"
)

// PluginDir is one installed plugin's skills directory + its stable ID.
type PluginDir struct {
	ID  string
	Dir string
}

// SkillPathInputs are the absolute filesystem anchors for skill discovery.
// HomeDir/CWD drive the default .claude/.opendbx roots; BuiltinDir is the
// optional bundled-skills root (empty in v1); PluginDirs are installed plugins.
type SkillPathInputs struct {
	HomeDir    string
	CWD        string
	BuiltinDir string
	PluginDirs []PluginDir
}

// skillsSubdir names the per-scope skill directory under .claude / .opendbx.
const skillsSubdir = "skills"

// BuildSkillRoots constructs discovery options from the path anchors and the
// plugin config. Roots get injective precedence via skills.AssignPrecedence
// (builtin < plugin < user < project < search-path; .opendbx outranks .claude
// within a scope). Config search paths are Required (an absent one is an
// error, unlike the always-optional defaults).
func BuildSkillRoots(in SkillPathInputs, cfg config.PluginsConfig) skills.DiscoverOptions {
	var specs []skills.RootSpec

	if in.BuiltinDir != "" {
		specs = append(specs, skills.RootSpec{
			Kind: skills.SourceBuiltin, Band: skills.BandBuiltin, SubOrder: 0,
			Dir: in.BuiltinDir,
		})
	}

	for i, pd := range in.PluginDirs {
		specs = append(specs, skills.RootSpec{
			Kind: skills.SourcePluginCache, Band: skills.BandPlugin, SubOrder: i,
			Dir: pd.Dir, PluginID: pd.ID,
		})
	}

	if in.HomeDir != "" {
		specs = append(specs,
			skills.RootSpec{Kind: skills.SourceUserGlobal, Band: skills.BandUser, SubOrder: skills.VariantClaude,
				Dir: filepath.Join(in.HomeDir, ".claude", skillsSubdir)},
			skills.RootSpec{Kind: skills.SourceUserGlobal, Band: skills.BandUser, SubOrder: skills.VariantOpendbx,
				Dir: filepath.Join(in.HomeDir, ".opendbx", skillsSubdir)},
		)
	}

	if in.CWD != "" {
		specs = append(specs,
			skills.RootSpec{Kind: skills.SourceProject, Band: skills.BandProject, SubOrder: skills.VariantClaude,
				Dir: filepath.Join(in.CWD, ".claude", skillsSubdir)},
			skills.RootSpec{Kind: skills.SourceProject, Band: skills.BandProject, SubOrder: skills.VariantOpendbx,
				Dir: filepath.Join(in.CWD, ".opendbx", skillsSubdir)},
		)
	}

	for i, p := range cfg.SkillSearchPaths {
		dir := p
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(in.CWD, dir)
		}
		specs = append(specs, skills.RootSpec{
			Kind: skills.SourceProject, Band: skills.BandSearchPath, SubOrder: i,
			Dir: filepath.Clean(dir), Required: true,
		})
	}

	return skills.DiscoverOptions{
		Roots:    skills.AssignPrecedence(specs),
		Disabled: cfg.DisabledSkills,
	}
}

// DiscoverSkills resolves the real home/cwd anchors and runs a one-shot
// discovery (spec-2.2 D-7 wiring; v1 has no builtin/plugin roots). It is the
// runnable entry the /debug-style lister calls.
func DiscoverSkills(cfg *config.Config) skills.DiscoveryResult {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	opts := BuildSkillRoots(SkillPathInputs{HomeDir: home, CWD: cwd}, cfg.Plugins)
	return skills.Discover(opts)
}

// DiscoverSkillsSummary runs discovery and formats it. Kept here (not in
// entrypoints) so the skills-package dependency stays in the app layer:
// entrypoints → bootstrap → app/skills is legal; entrypoints → app/skills is
// not (layer rule).
func DiscoverSkillsSummary(cfg *config.Config) string {
	return skills.SummarizeDiscovery(DiscoverSkills(cfg))
}
