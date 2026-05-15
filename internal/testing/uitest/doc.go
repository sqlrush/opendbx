// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package uitest provides a PTY + vt10x harness for cell-level UI tests.
// It is the base layer (Layer 2 of CLAUDE.md § 3.9 UI Review Pipeline);
// the visual-diff and AI-evaluator layers (Layer 3-4) build on top in
// spec-0.11.5.
//
// Capabilities:
//   - Term(t, cmd, cols, rows) — launch cmd inside a sized PTY
//     connected to a vt10x terminal emulator.
//   - CellGrid() / CellGridRunes() — read the cell grid as either
//     []string (UTF-8 with NBSP continuation) or [][]rune (one rune
//     per terminal column).
//   - Send(bytes) — write to the PTY (e.g., keystrokes).
//   - WaitFor(pred, timeout) — block until a predicate over the grid
//     holds, or fail.
//   - SnapshotGolden(name) — golden-compare the current cell grid.
//
// Platform: Unix-only. Tests in this package are excluded from Windows
// builds via //go:build !windows.
//
// Design: spec-0.11-test-framework § 1.1 D-4.
package uitest
