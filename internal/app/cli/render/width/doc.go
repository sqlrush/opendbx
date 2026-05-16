// Copyright 2026 opendbx contributors. See LICENSE.
//
// Package width is the single source of truth for rune-width measurement
// and column-aware string manipulation in opendbx's render subsystem.
//
// CLAUDE.md § 3.1 invariant: every call site that needs "visual column
// width" of a string must go through this package — direct calls to
// runewidth.StringWidth are forbidden by IMP-8 runewidth-wrap (only
// internal/testing/uiinvariant + internal/app/cli/render/width are
// whitelisted; spec-0.11.5 + spec-0.13).
//
// All measurements use runewidth.Condition with EastAsianWidth=false and
// StrictEmojiNeutral=true, matching the spec-0.11.5 widthCondition
// invariant.
//
// API (4 functions):
//   - Width(s) — visual column count
//   - Wrap(s, cols) — break into lines, each ≤ cols
//   - Truncate(s, max) — cut to ≤ max, append "…" if cut
//   - Pad(s, target) — right-pad with spaces to width == target
//
// Error contract: Wrap/Truncate/Pad panic with RENDER.INVALID_WIDTH
// when cols/max/target ≤ 0 (system-boundary fail-fast).
//
// DAG position: render/width is index 0 (leaf, no internal deps).
//
// Design: spec-0.13-render-engine-skeleton § 2.3 (D-2)
// Author: sqlrush
package width
