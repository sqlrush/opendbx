// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File summary.go — minimal discovery lister (spec-2.2 D-7). Plain stdlib
// formatting only: app/skills must stay a leaf (no render/table/color import).
// The /debug skills command wires this string into its output at the call site
// (entrypoints/diagnose), not here.

package skills

import (
	"fmt"
	"sort"
	"strings"
)

// SummarizeDiscovery renders a human-readable, deterministic summary of a
// discovery result: active skills, shadowed, conflicts, warnings, ignored, and
// errors. It does not invoke anything.
func SummarizeDiscovery(res DiscoveryResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "active (%d):\n", len(res.Active))
	for _, s := range res.Active {
		fmt.Fprintf(&b, "  %s  [%s prec=%d]  %s\n", s.Schema.Name, s.Source.Kind, s.Source.Precedence, s.Source.Path)
	}

	if len(res.Shadowed) > 0 {
		fmt.Fprintf(&b, "shadowed (%d):\n", len(res.Shadowed))
		for _, s := range res.Shadowed {
			fmt.Fprintf(&b, "  %s  [%s prec=%d]  %s\n", s.Schema.Name, s.Source.Kind, s.Source.Precedence, s.Source.Path)
		}
	}

	if len(res.Conflicts) > 0 {
		fmt.Fprintf(&b, "conflicts (%d):\n", len(res.Conflicts))
		for _, c := range res.Conflicts {
			kind := "shadowed"
			if c.Kind == ConflictUnresolvable {
				kind = "unresolvable"
			}
			fmt.Fprintf(&b, "  %s  [%s]  paths=%s\n", c.Name, kind, strings.Join(conflictPaths(c), ", "))
		}
	}

	if len(res.Warnings) > 0 {
		fmt.Fprintf(&b, "warnings (%d):\n", len(res.Warnings))
		for _, w := range res.Warnings {
			fmt.Fprintf(&b, "  %s  %s: %s\n", w.Path, w.Name, w.Warning.Detail)
		}
	}

	if len(res.Ignored) > 0 {
		fmt.Fprintf(&b, "ignored (%d):\n", len(res.Ignored))
		for _, ig := range res.Ignored {
			fmt.Fprintf(&b, "  %s  (%s)  %s\n", ig.Skill.Schema.Name, ig.Reason, ig.Skill.Source.Path)
		}
	}

	if len(res.Errors) > 0 {
		fmt.Fprintf(&b, "errors (%d):\n", len(res.Errors))
		for _, de := range res.Errors {
			fmt.Fprintf(&b, "  %s  [%s]\n", de.Path, ClassifyError(de))
		}
	}

	return b.String()
}

// conflictPaths returns the source paths of every skill in a conflict, sorted.
func conflictPaths(c Conflict) []string {
	paths := make([]string, 0, len(c.Shadowed)+1)
	if c.Winner != nil {
		paths = append(paths, c.Winner.Source.Path)
	}
	for _, s := range c.Shadowed {
		paths = append(paths, s.Source.Path)
	}
	sort.Strings(paths)
	return paths
}
