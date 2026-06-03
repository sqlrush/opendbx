// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File skill.go — Skill value type + SkillSource provenance + stable identity
// (spec-2.1 D-2). Skill is immutable: Parse returns a fresh value and nothing
// here mutates inputs (规则 12).

package skills

// SourceKind describes where a skill was discovered. It is descriptive;
// SkillSource.Precedence is the authoritative resolution key (review HIGH-4:
// 2.2 interleaves dual .claude/.opendbx + plugin-cache by assigning
// Precedence per its own path-ranking policy, not by re-ranking this enum).
type SourceKind int

const (
	// SourceUnknown is the zero value. Resolve treats it as the lowest
	// precedence and is the default when 2.2 has not tagged a skill.
	SourceUnknown SourceKind = iota
	// SourceBuiltin — opendbx built-in skill.
	SourceBuiltin
	// SourcePluginCache — installed plugin pack (.claude/plugins/cache/...).
	SourcePluginCache
	// SourceUserGlobal — ~/.claude or ~/.opendbx.
	SourceUserGlobal
	// SourceProject — ./.claude or ./.opendbx (highest by convention).
	SourceProject
)

// String renders the SourceKind for diagnostics.
func (k SourceKind) String() string {
	switch k {
	case SourceBuiltin:
		return "builtin"
	case SourcePluginCache:
		return "plugin-cache"
	case SourceUserGlobal:
		return "user-global"
	case SourceProject:
		return "project"
	default:
		return "unknown"
	}
}

// SkillSource carries discovery provenance. spec-2.2 fills the real values;
// spec-2.1 only defines the shape and the precedence contract.
type SkillSource struct {
	Kind       SourceKind
	Precedence int    // higher wins in Resolve; 2.2 assigns per path policy
	Path       string // source file path (error messages + reporting)
	PluginID   string // non-empty for SourcePluginCache
}

// Skill is a fully parsed SKILL.md: frontmatter + body + provenance.
type Skill struct {
	Schema Schema
	Body   string // markdown body, byte-faithful after the closing fence
	Source SkillSource
}

// Key returns the canonical downstream identity, which is the validated
// Schema.Name. spec-2.3 maps a tool name to Skill.Key(). The loose name rule
// (Q5) forbids path/control chars but does NOT force lowercase, so callers
// compare keys byte-exact (case-sensitive).
func (s Skill) Key() string { return s.Schema.Name }
