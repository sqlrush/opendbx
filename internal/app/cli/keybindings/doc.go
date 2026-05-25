// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package keybindings maps low-level key events to high-level Actions
// (spec-1.17 D-1). It is the input-side dispatcher between
// render/terminal events and program/Model semantics.
//
// DAG position: keybindings is at 9.6 — it imports render/terminal (2)
// only. It does NOT import input (9.5) — see input/MovementKind for the
// cursor-movement enum on the other side of the boundary. program (10)
// imports keybindings; demoapp / spec-1.20 Models also import this
// package to switch on ActionXxx values returned by program.handleMsg
// after decoding.
//
// Three invariants (spec-1.17 R2):
//
//  1. Key code SoT — render/terminal/driver.go is the single source of
//     truth for terminal.Key* int values. program.KeyMsg.Code and
//     keybindings.KeyEvent.Code are thin re-exports. Adding a new Key
//     follows: terminal/driver.go → render/terminal/tcell adapter →
//     keybindings/defaultBindings.go (no enum duplication).
//
//  2. Action append-only — reorder / delete on the Action const block
//     is BREAKING. spec-2.1 ActionTab / spec-2.4 ActionComplete /
//     spec-2.x ActionVimMode etc. MUST append to the end.
//
//  3. Reserved minimal — spec-1.17 hard-Reserved = {ActionQuit (Ctrl+C)}
//     only. CC reserves {Ctrl+C, Ctrl+D, Ctrl+M}; opendbx defers
//     Ctrl+D/M to spec-2.x reservedShortcuts overlay (AD-005 deviation).
//
// CC parity (B-51..B-55 in spec-1.17 § 1.4): this package is a minimal
// subset of CC's src/keybindings/ — no schema parser, no user overlay.
// loadUserBindings overlay lands in spec-2.x.
package keybindings
