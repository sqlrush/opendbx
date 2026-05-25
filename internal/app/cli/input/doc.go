// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package input is the spec-1.16 three-mode input primitives:
//   - Mode enum (ModeNatural / ModeSlash / ModeSQL)
//   - DeriveMode(buffer) — SOLE source of truth via Buffer[0]; mode is
//     NEVER stored as state (spec-1.16 R2 C2 ★A 路径 A by-construction
//     single SoT)
//   - ResolveMode(buffer, cursor, code, r) — pure function applying one
//     keypress (KeyRune append / KeyBackspace delete) at cursor==end;
//     mid-cursor + KeyDelete deferred to spec-1.17 (❌-10)
//   - ValueWithoutPrefix(buffer) — strip mode trigger rune for downstream
//     slash registry / SQL parser consumption (spec-2.1 / spec-2.4)
//   - StyleFor(mode) — input-local mode→style.Style resolver (DAG-isolated
//     from render/block.StyleKind; spec-1.16 R3 codex MED-4 fix: prior
//     exported StyleKind enum removed, no external consumer)
//
// DAG position: 9.5 (between render/streaming index 9 and
// app/cli/program index 10). input imports only render/terminal (index
// 2) for KeyRune/KeyBackspace constants and render/style for Style. It
// does NOT import program — program imports input one-way (spec-1.16 R3
// codex CRIT fix; import cycle resolved by signature taking int+rune
// primitives instead of program.KeyMsg).
//
// Design: spec-1.16-input-three-modes.md
package input
