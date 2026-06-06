// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package invoke adapts discovered skills (spec-2.2) to the diagnose
// loop tool surface (spec-1.21). Design: spec-2.3-skill-invocation.
//
// It deliberately imports BOTH app/skills and app/diagnose — the skills
// core package stays a pure leaf (spec-2.2 D-9 invariant: app/skills
// must not import diagnose/config/logger/render, and must not import
// this subpackage either; spec-2.3 D-8 directional rule).
//
// Naming note (spec-2.3 Q3 / R-8): the tool's wire name is "Skill"
// (capitalized) for 1:1 Claude Code parity — a deliberate divergence
// from the lowercase clock/echo convention, which only applies to
// opendbx-native tools.
package invoke
