// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package uiinvariant provides Layer 1 static invariant checks for
// terminal UI testing — the millisecond-level lane of spec-0.11.5
// 5-layer UI Review Pipeline (CLAUDE.md § 3.9).
//
// Helpers:
//   - CheckRowWidth — assert every cell-grid row fits within column
//     budget (runewidth EastAsianWidth=false).
//   - CheckANSI — state-based SGR validator; tracks residual active
//     attributes at stream end. Covers 0m universal reset, 22m/23m/
//     24m/27m/39m/49m targeted resets, 30-37/40-47 standard colors,
//     90-97/100-107 bright colors, 38;5;N/48;5;N 256-color, and
//     38;2;R;G;B/48;2;R;G;B 24-bit color.
//
// Static checks have no PTY / no goroutines / no build-tag constraints.
// Cross-platform (Windows-safe).
//
// Cell-metadata (vt10x.Terminal.Cell vs CellGrid consistency) belongs
// in the uitest package — see uitest.Terminal.CheckCellMetadata.
//
// Design: spec-0.11.5-ui-review-pipeline § D-1.
package uiinvariant
