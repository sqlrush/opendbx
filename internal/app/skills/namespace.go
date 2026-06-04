// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File namespace.go — namespace resolution (spec-2.1 D-5). Pure, no I/O.
//
// Precondition: inputs have passed Validate (non-empty, well-formed Name).
// Skills with an empty Name are segregated — they never group or reach Active
// (review HIGH-3: prevents empty names from collapsing into a false conflict).
//
// Resolution model:
//   - Same Name across DIFFERENT precedence → ConflictShadowed: the highest
//     precedence wins (mirrors CC "project overrides global"); losers are
//     reported, not silently dropped.
//   - Same Name AND SAME precedence → ConflictUnresolvable: no winner is
//     chosen (规则 7 / Q8 — don't silently pick); 2.2/UI decides.
//
// Resolve never mutates the input slice or the Skills' Extra maps (规则 12).

package skills

import (
	"sort"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// ConflictKind distinguishes a legal shadow from an unresolvable duplicate.
type ConflictKind int

const (
	// ConflictShadowed — a higher-precedence skill won; lower ones shadowed.
	ConflictShadowed ConflictKind = iota
	// ConflictUnresolvable — two+ skills share a name at the same precedence.
	ConflictUnresolvable
)

// Conflict records one name collision.
type Conflict struct {
	Name     string
	Kind     ConflictKind
	Winner   *Skill  // the active skill for ConflictShadowed; nil if Unresolvable
	Shadowed []Skill // the losers (Shadowed) or the full ambiguous set (Unresolvable)
}

// Err returns a SKILL.NAMESPACE_CONFLICT error for an unresolvable conflict,
// or nil for a legally resolved shadow. spec-2.2 calls this when it chooses to
// fail on ambiguous skills (spec-2.1 only reports them as data).
func (c Conflict) Err() error {
	if c.Kind != ConflictUnresolvable {
		return nil
	}
	return errcode.Newf(ErrNamespaceConflict.Code(),
		"skill %q is defined %d times at the same precedence", c.Name, len(c.Shadowed))
}

// Resolution is the outcome of Resolve.
type Resolution struct {
	Active    []Skill    // one winner per name (sorted by name)
	Shadowed  []Skill    // every skill a winner shadowed
	Conflicts []Conflict // every name with >1 skill (Shadowed or Unresolvable)
}

// Resolve picks one active skill per name by precedence and reports conflicts.
// See the file comment for the resolution model.
func Resolve(skills []Skill) Resolution {
	groups, order := groupByName(skills)
	var res Resolution
	for _, name := range order {
		group := groups[name]
		if len(group) == 1 {
			res.Active = append(res.Active, cloneSkill(group[0]))
			continue
		}
		top, tie := topByPrecedence(group)
		if tie {
			// Unresolvable: do not pick a winner.
			res.Conflicts = append(res.Conflicts, Conflict{
				Name:     name,
				Kind:     ConflictUnresolvable,
				Winner:   nil,
				Shadowed: cloneSkillSlice(group),
			})
			continue
		}
		winner := cloneSkill(group[top])
		losers := make([]Skill, 0, len(group)-1)
		for i, s := range group {
			if i != top {
				losers = append(losers, cloneSkill(s))
			}
		}
		res.Active = append(res.Active, winner)
		res.Shadowed = append(res.Shadowed, losers...)
		w := winner
		res.Conflicts = append(res.Conflicts, Conflict{
			Name:     name,
			Kind:     ConflictShadowed,
			Winner:   &w,
			Shadowed: losers,
		})
	}
	return res
}

// DetectConflicts returns the conflicts Resolve would report, without building
// the Active/Shadowed sets. Useful for 2.2 pre-flight reporting.
func DetectConflicts(skills []Skill) []Conflict {
	return Resolve(skills).Conflicts
}

// groupByName buckets skills by Name (skipping empty names) and returns the
// buckets plus a name list sorted for deterministic output. Input order within
// a bucket is preserved.
func groupByName(skills []Skill) (map[string][]Skill, []string) {
	groups := make(map[string][]Skill)
	for _, s := range skills {
		if s.Schema.Name == "" {
			continue // segregated: invalid input, never grouped
		}
		groups[s.Schema.Name] = append(groups[s.Schema.Name], s)
	}
	order := make([]string, 0, len(groups))
	for name := range groups {
		order = append(order, name)
	}
	sort.Strings(order)
	return groups, order
}

// topByPrecedence returns the index of the single highest-precedence skill in
// group and whether the top is tied (two+ at the max precedence).
func topByPrecedence(group []Skill) (idx int, tie bool) {
	best := 0
	count := 1
	for i := 1; i < len(group); i++ {
		switch {
		case group[i].Source.Precedence > group[best].Source.Precedence:
			best = i
			count = 1
		case group[i].Source.Precedence == group[best].Source.Precedence:
			count++
		}
	}
	return best, count > 1
}

// cloneSkillSlice deep-copies every Skill so the result shares no Extra map
// with the input (review HIGH: Resolve output must not alias caller input).
func cloneSkillSlice(in []Skill) []Skill {
	out := make([]Skill, len(in))
	for i, s := range in {
		out[i] = cloneSkill(s)
	}
	return out
}

// cloneSkill returns a copy of s with a deep-copied Schema.Extra so a caller
// mutating the returned skill cannot reach back into the input (规则 12).
func cloneSkill(s Skill) Skill {
	s.Schema.Extra = deepCopyExtra(s.Schema.Extra)
	return s
}

// deepCopyExtra recursively copies a frontmatter Extra map (values are the
// map[string]any / []any / scalar shapes yaml.Node.Decode produces).
func deepCopyExtra(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyValue(v)
	}
	return out
}

func deepCopyValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return deepCopyExtra(t)
	case []any:
		s := make([]any, len(t))
		for i, e := range t {
			s[i] = deepCopyValue(e)
		}
		return s
	default:
		return v // scalars are immutable
	}
}
