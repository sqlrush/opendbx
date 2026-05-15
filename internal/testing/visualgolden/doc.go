// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package visualgolden implements Layer 3 of the spec-0.11.5 UI Review
// Pipeline: freeze (ANSI → PNG) + self-implemented pixel diff with YIQ
// luminance distance.
//
// Workflow:
//  1. Capture ANSI bytes from a uitest.Terminal (Terminal.ANSIRaw).
//  2. visualgolden.Render(t, ansi, theme) — invokes pinned freeze
//     v0.2.2 binary to produce a deterministic PNG. Requires
//     librsvg2-bin + fonts-jetbrains-mono + fonts-noto-cjk on host;
//     verified at CI install step (see spec § 5.1).
//  3. visualgolden.Compare(t, name, got, maxMismatchFraction) — diffs
//     against testdata/visual/<TestName>[/<sub>].png golden; uses
//     pixelSensitivity YIQ-distance for per-pixel decision; reports
//     fraction of differing pixels. On -update-visual flag, writes
//     golden + sidecar metadata.json (font/freeze/rsvg versions).
//
// Metadata sidecar (spec § 2.3 R4 codex HIGH-3): goldens carry a
// metadata.json with the rendering toolchain version. Compare verifies
// metadata first; drift fails before any pixel diff to avoid false
// positives from font/converter upgrades.
//
// CI requirement (spec § 5.1 + § D-5 ui-visual job; user CRIT-1 R5):
// VISUALGOLDEN_REQUIRED=1 promotes Skipf → Fatalf when freeze binary
// is missing. CI always sets it; dev local doesn't.
//
// Design: spec-0.11.5-ui-review-pipeline § D-2.
package visualgolden
