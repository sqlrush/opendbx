// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package tcell is the tcell-backed terminal.Driver adapter (spec-1.17
// D-6a). It is the single concentration point for tcell event/keycode
// translation in production opendbx code paths.
//
// Layering:
//
//   - render/terminal (parent) defines the tcell-free Driver interface
//     and terminal.Event types.
//   - render/terminal/tcell (this package) implements Driver over a
//     tcell.Screen. It owns Init/Fini lifecycle (via sync.Once for Fini
//     idempotency) and the tcell.Event ↔ terminal.Event mapping table.
//   - program / demoapp / scheduler MUST NOT import tcell — they consume
//     only the terminal.Driver interface. IMP-9 tcell isolation enforces
//     this via the 4-package whitelist (terminal + tui + bootstrap +
//     this package).
//
// Key normalization (spec-1.17 R-10):
//
//   - tcell.KeyBackspace (8) → terminal.KeyBackspace
//   - tcell.KeyBackspace2 (127 = ASCII DEL) → terminal.KeyBackspace
//     (lossy normalize: most terminal stty erase = backspace, BS/DEL
//     both delete-left; forward delete uses KeyDelete=271 instead)
//   - tcell.KeyDelete (271) → terminal.KeyDelete
//
// Lifecycle ownership (spec-1.17 D-6a R3):
//
//   - Caller (bootstrap) constructs a tcell.Screen via tcell.NewScreen
//     factory but does NOT Init it.
//   - Caller wraps the screen with NewDriver(screen).
//   - scheduler.Run (driven by program.Run) calls Driver.Init() →
//     screen.Init() and Driver.Fini() → screen.Fini() via sync.Once.
//   - Caller MUST NOT defer screen.Fini() — Driver.Fini owns it.
package tcell
