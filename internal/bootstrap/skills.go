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
	"sort"

	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/app/skills"
	"github.com/sqlrush/opendbx/internal/app/skills/invoke"
	"github.com/sqlrush/opendbx/internal/platform/config"
	"github.com/sqlrush/opendbx/internal/platform/logger"
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

	// Sort plugins by ID (then Dir) so SubOrder — and thus cross-plugin
	// same-name precedence — is deterministic regardless of caller order
	// (locked Q8). A copy keeps the input slice immutable.
	plugins := append([]PluginDir(nil), in.PluginDirs...)
	sort.Slice(plugins, func(i, j int) bool {
		if plugins[i].ID != plugins[j].ID {
			return plugins[i].ID < plugins[j].ID
		}
		return plugins[i].Dir < plugins[j].Dir
	})
	for i, pd := range plugins {
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
//
// A failed UserHomeDir/Getwd yields an empty anchor: the corresponding default
// roots are simply skipped (they are optional, so absent → empty), and any
// relative skill_search_paths then become unanchored Required roots that
// surface as SKILL.ROOT_UNREADABLE in the result — observable, not silent.
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

// bodySizeWarnBytes is the soft warning threshold for a skill body
// injected into the transcript (spec-2.3 R-7: a huge body eats the
// context budget on a single invoke; the hard cutoff is spec-3.10).
const bodySizeWarnBytes = 64 << 10

// skillsForChat adapts a discovery result for the chat model (spec-2.3
// D-5): the SkillTool executor plus the system-prompt skills section.
//
// Failure model (user decision 4/4, 2026-06-05): a NewSkillTool error —
// an upstream spec-2.2 contract violation — is LOGGED and skills are
// skipped for the session; interact always continues. Never panics.
// 0 active skills → no executor, no prompt section (spec-2.3 Q6).
func skillsForChat(res skills.DiscoveryResult) (execs []diagnose.ToolExecutor, systemPrompt string) {
	logSkillDiscovery(res)
	if len(res.Active) == 0 {
		return nil, ""
	}
	st, err := invoke.NewSkillTool(res.Active)
	if err != nil {
		logger.WarnForceFile(
			"skill tool construction failed; skills are disabled for this session",
			"spec", "2.3", "deliverable", "D-5", "err", err.Error(),
		)
		return nil, ""
	}
	section := skills.PromptSection(res.Active)
	logger.InfoForceFile( // normal path — info band, not warn (post-impl 双路命中)
		"skills active for this session",
		"spec", "2.3", "active", len(res.Active), "prompt_section_bytes", len(section),
	)
	return []diagnose.ToolExecutor{st}, section
}

// logSkillDiscovery surfaces discovery problems and per-skill trust
// warnings in the debug log (规则 7 — never silent; file-only so the
// TUI cell grid is not torn). Covers spec-2.3 R-10 (plugin-cache
// provenance) and R-7/R-11 (oversized body) at startup — v1 discovery
// runs once, so bodies and sources are static for the session.
func logSkillDiscovery(res skills.DiscoveryResult) {
	if n := len(res.Errors); n > 0 {
		logger.WarnForceFile("skill discovery reported errors (run /debug skills)",
			"spec", "2.3", "errors", n)
	}
	if n := len(res.Warnings); n > 0 {
		logger.WarnForceFile("skill discovery reported warnings (run /debug skills)",
			"spec", "2.3", "warnings", n)
	}
	for _, sk := range res.Active {
		if sk.Source.Kind == skills.SourcePluginCache {
			logger.WarnForceFile("active skill comes from the plugin cache; its body enters the model context verbatim",
				"spec", "2.3", "risk", "R-10", "skill", sk.Key(), "plugin", sk.Source.PluginID, "path", sk.Source.Path)
		}
		if len(sk.Body) > bodySizeWarnBytes {
			logger.WarnForceFile("active skill body exceeds the 64KiB soft cap; one invoke will consume significant context",
				"spec", "2.3", "risk", "R-7", "skill", sk.Key(), "body_bytes", len(sk.Body))
		}
	}
}
