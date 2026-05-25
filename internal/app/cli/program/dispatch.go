// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package program

import (
	"github.com/sqlrush/opendbx/internal/app/cli/input"
	"github.com/sqlrush/opendbx/internal/app/cli/keybindings"
)

// KeyActionMsg pairs a raw KeyMsg with the keybindings.Action decoded
// by program.handleMsg. spec-1.17 D-5: program is the single decode
// point for keys → Actions; Models switch on KeyActionMsg.Action.
//
// ── Invariant — Action consume contract (spec-1.17 R2 HIGH-6) ─────────
// Models MUST consume KeyActionMsg.Action; KeyActionMsg.Key is fallback
// / log only. Models MUST NOT re-invoke keybindings.Resolve(msg.Key) —
// spec-2.x user customization layers binding overlays in program, and a
// Model that re-decodes would silently drop the user overrides.
//
// Models that want to inspect Action.String() or compare ActionXxx
// constants MAY import keybindings; only Resolve is forbidden from
// Models (Q7 narrow, spec-1.17 R2 R3 absorb).
type KeyActionMsg struct {
	Key    KeyMsg
	Action keybindings.Action
}

// actionToMovement is the program-layer translator from
// keybindings.Action to input.MovementKind. This is the single
// boundary that satisfies the DAG forward-only contract:
//
//	input(9.5) ←─ program(10) ─→ keybindings(9.6)
//
// input and keybindings do not import each other; program imports both
// and bridges. spec-1.17 R2 D-3 + CRIT-2 absorb.
func actionToMovement(a keybindings.Action) input.MovementKind {
	switch a {
	case keybindings.ActionMoveLeft:
		return input.MoveLeft
	case keybindings.ActionMoveRight:
		return input.MoveRight
	case keybindings.ActionMoveHome:
		return input.MoveHome
	case keybindings.ActionMoveEnd:
		return input.MoveEnd
	}
	return input.MoveNone
}

// decodeKeyMsg wraps keybindings.Resolve in a program-internal helper so
// the call site is a single line.
func decodeKeyMsg(k KeyMsg) keybindings.Action {
	return keybindings.Resolve(keybindings.KeyEvent{
		Code: k.Code,
		Rune: k.Rune,
		Mod:  k.Mod,
	})
}
