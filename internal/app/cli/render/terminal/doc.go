// Copyright 2026 opendbx contributors. See LICENSE.
//
// Package terminal is the render-subsystem driver abstraction. It hides
// the tcell.Screen / tcell.Event / tcell.Key / tcell.ModMask surface
// from the rest of opendbx, so upper render layers (buffer, layout,
// block, scrollback, streaming) and cmd layer never import tcell.
//
// IMP-9 tcell-isolation whitelist (spec-0.13 D-5 brings to 4 packages):
// internal/platform/terminal (Probe) + internal/app/cli/tui (NewScreen)
// + internal/bootstrap (LaunchInteractiveTUI test seam) +
// internal/app/cli/render/terminal (this package: Driver).
//
// Event marker interface uses tcell-free primitives (Code int keypress
// + ShiftCtrlAlt uint8 3-bit modifier) — see types.go.
//
// DAG position: render/terminal is index 2 (depends on render/style).
//
// Design: spec-0.13-render-engine-skeleton § 2.1 (D-1) + § 2.5 (D-5)
// Author: sqlrush
package terminal
