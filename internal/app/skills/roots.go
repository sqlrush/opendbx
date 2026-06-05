// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File roots.go — skill root model + total-injective precedence (spec-2.2 D-1).
//
// Precedence must be a TOTAL INJECTIVE function over every root — the fixed
// scopes (builtin/user/project × .claude/.opendbx) AND the dynamic ones (N
// plugin caches, M config search paths). AssignPrecedence guarantees this by
// sorting RootSpecs into a canonical low→high order and handing each a
// sequential integer, so no two roots ever tie (review HIGH-3). Cross-root
// same-name then always resolves to a deterministic Shadowed; only two files
// of the same name WITHIN one root tie (→ Unresolvable, surfaced as duplicate).

package skills

import "sort"

// defaultMaxFilesPerRoot caps how many skill files one root may contribute
// before the scan truncates and reports SKILL.ROOT_TOO_MANY_FILES (DoS guard).
const defaultMaxFilesPerRoot = 1000

// Precedence bands (low→high). A skill from a higher band wins. Bands are
// descriptive; AssignPrecedence collapses (band, subOrder, dir) into a dense
// injective rank, so band spacing never needs to leave room for sub-orders.
// Exported so bootstrap (BuildSkillRoots) can place roots on the ladder.
const (
	BandBuiltin    = 0
	BandPlugin     = 1
	BandUser       = 2
	BandProject    = 3
	BandSearchPath = 4
)

// Dual-path variant sub-order within a scope: .opendbx outranks .claude
// (Q3 — opendbx host overrides the portable CC source).
const (
	VariantClaude  = 0
	VariantOpendbx = 1
)

// RootSpec describes a skill root before precedence assignment. bootstrap
// builds these from HomeDir/cwd/plugins/searchPaths; AssignPrecedence ranks
// them. SubOrder is the deterministic within-band tiebreak (dual-path variant,
// sorted plugin index, or config-list index).
type RootSpec struct {
	Kind     SourceKind
	Band     int
	SubOrder int
	Dir      string
	PluginID string
	Required bool // absent Required root → SKILL.ROOT_UNREADABLE; absent optional → empty
}

// SkillRoot is a precedence-ranked directory to scan.
type SkillRoot struct {
	Kind       SourceKind
	Precedence int // injective; higher wins
	Dir        string
	PluginID   string
	Required   bool
}

// DiscoverOptions is the input to Discover. Roots carry injective Precedence
// (from AssignPrecedence). Disabled names are matched by Skill.Key()
// (case-sensitive). MaxFilesPerRoot 0 → defaultMaxFilesPerRoot.
type DiscoverOptions struct {
	Roots           []SkillRoot
	Disabled        []string
	MaxFilesPerRoot int
}

// AssignPrecedence ranks specs into SkillRoots with a dense injective
// Precedence. Order key: (Band, SubOrder, Dir) — Dir is the final tiebreak so
// the ranking is deterministic regardless of input order. Two specs sharing
// all three keys (the same directory listed twice) collapse to the SAME
// Precedence, which is correct: a single root is one precedence level.
func AssignPrecedence(specs []RootSpec) []SkillRoot {
	ordered := make([]RootSpec, len(specs))
	copy(ordered, specs)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.Band != b.Band {
			return a.Band < b.Band
		}
		if a.SubOrder != b.SubOrder {
			return a.SubOrder < b.SubOrder
		}
		return a.Dir < b.Dir
	})
	out := make([]SkillRoot, len(ordered))
	for i, s := range ordered {
		out[i] = SkillRoot{
			Kind:       s.Kind,
			Precedence: i, // dense, injective; higher index = higher precedence
			Dir:        s.Dir,
			PluginID:   s.PluginID,
			Required:   s.Required,
		}
	}
	return out
}
