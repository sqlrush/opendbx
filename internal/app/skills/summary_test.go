// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package skills

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// TestSummarizeDiscovery_AllSections exercises every render branch (active,
// shadowed, conflicts incl conflictPaths, warnings, ignored, errors) so a
// formatting regression in any section is caught (review go MED-2).
func TestSummarizeDiscovery_AllSections(t *testing.T) {
	t.Parallel()
	mk := func(name, path string, prec int) Skill {
		return Skill{Schema: Schema{Name: name}, Source: SkillSource{Kind: SourceProject, Precedence: prec, Path: path}}
	}
	winner := mk("dup", "/hi/dup.md", 2)
	res := DiscoveryResult{
		Active:   []Skill{mk("alpha", "/a/alpha.md", 1)},
		Shadowed: []Skill{mk("dup", "/lo/dup.md", 1)},
		Conflicts: []Conflict{{
			Name: "dup", Kind: ConflictShadowed, Winner: &winner,
			Shadowed: []Skill{mk("dup", "/lo/dup.md", 1)},
		}, {
			Name: "amb", Kind: ConflictUnresolvable,
			Shadowed: []Skill{mk("amb", "/x/amb.md", 5), mk("amb", "/x/amb2.md", 5)},
		}},
		Warnings: []DiscoveryWarning{{Path: "/a/w.md", Name: "warned", Warning: Warning{Detail: "non-kebab"}}},
		Ignored:  []IgnoredSkill{{Skill: mk("off", "/a/off.md", 1), Reason: "disabled"}},
		Errors:   []DiscoveryError{{Path: "/bad/x.md", Err: errcode.New(ErrParseError.Code(), "", "")}},
	}
	out := SummarizeDiscovery(res)
	for _, want := range []string{
		"active (1)", "alpha",
		"shadowed (1)",
		"conflicts (2)", "unresolvable", "/x/amb.md",
		"warnings (1)", "non-kebab",
		"ignored (1)", "off", "disabled",
		"errors (1)", "SKILL.PARSE_ERROR",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q\n---\n%s", want, out)
		}
	}
}
