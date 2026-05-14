// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// IMP-5 opendb-ban: forbid imports of opendb (the predecessor project)
// modules from opendbx (the greenfield rewrite per AD-001).
//
// Rationale (CLAUDE.md 原则 1 + § 6 禁忌 1, AD-001):
//
// opendbx is a Greenfield rewrite of opendb. opendb code is the "lessons
// learned" reference (docs/surveys/survey-opendb-lessons-learned.md) but
// MUST NOT be linked into opendbx source. Even accidentally importing an
// opendb subpackage as a transitive dep would smuggle in patterns the
// team explicitly decided to drop.
//
// This rule scans every import edge from opendbx code and rejects any
// path whose module prefix is in OpendbBannedPrefixes.

package rules

import (
	"fmt"
	"strings"
)

// OpendbBannedPrefixes are the module path prefixes forbidden in opendbx
// imports. Add entries as new opendb-hosted module names surface.
// spec-0.10 D-3 IMP-5 / R2 codex MED-2: enumerate explicit list.
var OpendbBannedPrefixes = []string{
	"github.com/opendb/",
	"github.com/opendb-project/",
	"bitbucket.org/opendb/",
	"gitlab.com/opendb/",
	"gitee.com/opendb/",
}

// CheckOpendbBan returns "" if the from→to edge is allowed, or a
// violation description if `to` is under an opendb-banned prefix.
//
// `from` is checked for completeness but the rule only inspects `to`
// (any opendbx code importing opendb is forbidden regardless of where
// inside opendbx the import sits).
func CheckOpendbBan(_, to string) string {
	for _, prefix := range OpendbBannedPrefixes {
		if strings.HasPrefix(to, prefix) {
			return fmt.Sprintf(
				"IMP-5 opendb-ban: import %q matches forbidden opendb prefix %q (AD-001 Greenfield rewrite; opendb is reference-only)",
				to, prefix)
		}
	}
	return ""
}
