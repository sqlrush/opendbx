// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File plugins.go — PluginsConfig (spec-2.2 D-6). Skill discovery configuration:
// extra search paths + disabled skill names. The actual filesystem root
// derivation lives in bootstrap (BuildSkillRoots); config only holds + validates
// the declarative values.

package config

import (
	"path/filepath"
	"strconv"
	"strings"
)

// maxSkillSearchPaths caps how many extra skill roots a config may declare
// (DoS / footgun guard).
const maxSkillSearchPaths = 10

// PluginsConfig declares skill-discovery inputs (spec-2.2). Extension point for
// future plugin features; only skill fields are populated now (§3.7 append-only).
type PluginsConfig struct {
	// SkillSearchPaths are extra directories to scan for SKILL.md, beyond the
	// default ~/.claude, ~/.opendbx, ./.claude, ./.opendbx roots.
	SkillSearchPaths []string `yaml:"skill_search_paths,omitempty" json:"skill_search_paths,omitempty"`
	// DisabledSkills are skill names (Skill.Key()) to exclude from discovery.
	DisabledSkills []string `yaml:"disabled_skills,omitempty" json:"disabled_skills,omitempty"`
}

// validatePlugins checks the plugin/skill discovery config. Rejects lexical
// path traversal (a `..` component that escapes after Clean) in search paths
// and caps their count. Path canonicalisation (Abs/Clean) happens later in
// bootstrap with the real cwd, and discovery rejects a symlinked root dir
// (scanRoot Lstat).
//
// v1 scope (single-user): this lexical guard + the symlink-root rejection
// cover the realistic threats. Full realpath containment (EvalSymlinks +
// cwd-subtree enforcement for project-sourced paths, blocking intermediate
// symlink components) is deferred to the enterprise/multi-user deployment
// (spec-3.9), where untrusted shared config is the threat model.
func validatePlugins(cfg *Config, errs *ValidationErrors) {
	if cfg == nil {
		return
	}
	src := cfg.Source("Plugins").String()
	paths := cfg.Plugins.SkillSearchPaths

	if len(paths) > maxSkillSearchPaths {
		*errs = append(*errs, ValidationError{
			Path: "Plugins.SkillSearchPaths", Rule: "max",
			Expected: "at most " + strconv.Itoa(maxSkillSearchPaths) + " entries",
			Actual:   strconv.Itoa(len(paths)), Source: src,
		})
	}
	for i, p := range paths {
		if hasDotDot(p) {
			*errs = append(*errs, ValidationError{
				Path:     "Plugins.SkillSearchPaths[" + strconv.Itoa(i) + "]",
				Rule:     "no-traversal",
				Expected: "no .. path component (traversal guard)",
				Actual:   p, Source: src,
			})
		}
	}
}

// hasDotDot reports whether a path contains a ".." component (before or after
// cleaning), guarding against directory traversal.
func hasDotDot(p string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(p))
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}
