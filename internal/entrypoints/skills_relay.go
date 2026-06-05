// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Skills relay (spec-2.2 D-7). Routes cmd/opendbx → bootstrap.DiscoverSkills +
// skills.SummarizeDiscovery so cmd keeps its single platform exception
// (version only). Minimal lister surface; the full /skills command tree is
// spec-2.18.

package entrypoints

import "github.com/sqlrush/opendbx/internal/bootstrap"

// SkillsSummary loads the default config, runs a one-shot skill discovery, and
// returns a human-readable summary (active/shadowed/conflicts/warnings/
// ignored/errors). It performs no invocation. The discovery + formatting live
// in bootstrap so entrypoints does not import app/skills directly (layer rule).
func SkillsSummary() (string, error) {
	cfg, err := LoadConfigDefault()
	if err != nil {
		return "", err
	}
	return bootstrap.DiscoverSkillsSummary(cfg), nil
}
