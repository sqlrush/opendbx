// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package program

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/input"
	"github.com/sqlrush/opendbx/internal/app/cli/keybindings"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// TestActionToMovement covers all 5 Action ↔ MovementKind mappings + the
// "other Actions → MoveNone" defensive default (spec-1.17 R2 D-3).
func TestActionToMovement(t *testing.T) {
	t.Parallel()
	cases := []struct {
		action keybindings.Action
		want   input.MovementKind
	}{
		{keybindings.ActionMoveLeft, input.MoveLeft},
		{keybindings.ActionMoveRight, input.MoveRight},
		{keybindings.ActionMoveHome, input.MoveHome},
		{keybindings.ActionMoveEnd, input.MoveEnd},
		// Non-movement Actions all collapse to MoveNone.
		{keybindings.ActionInsertRune, input.MoveNone},
		{keybindings.ActionDeleteBackward, input.MoveNone},
		{keybindings.ActionDeleteForward, input.MoveNone},
		{keybindings.ActionHistoryPrev, input.MoveNone},
		{keybindings.ActionHistoryNext, input.MoveNone},
		{keybindings.ActionSubmit, input.MoveNone},
		{keybindings.ActionCancel, input.MoveNone},
		{keybindings.ActionQuit, input.MoveNone},
		{keybindings.ActionNone, input.MoveNone},
	}
	for _, tc := range cases {
		t.Run(tc.action.String(), func(t *testing.T) {
			if got := actionToMovement(tc.action); got != tc.want {
				t.Errorf("actionToMovement(%v) = %v; want %v", tc.action, got, tc.want)
			}
		})
	}
}

// TestDecodeKeyMsg verifies the program-internal helper routes a KeyMsg
// through keybindings.Resolve to produce the expected Action.
func TestDecodeKeyMsg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		k    KeyMsg
		want keybindings.Action
	}{
		{"KeyRune 'a' → InsertRune", KeyMsg{Code: terminal.KeyRune, Rune: 'a'}, keybindings.ActionInsertRune},
		{"KeyUp → HistoryPrev", KeyMsg{Code: terminal.KeyUp}, keybindings.ActionHistoryPrev},
		{"KeyDelete → DeleteForward", KeyMsg{Code: terminal.KeyDelete}, keybindings.ActionDeleteForward},
		{"unknown code → None", KeyMsg{Code: 9999}, keybindings.ActionNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decodeKeyMsg(tc.k); got != tc.want {
				t.Errorf("decodeKeyMsg(%+v) = %v; want %v", tc.k, got, tc.want)
			}
		})
	}
}

// TestKeyActionMsg_FieldsPreserveKey ensures the raw KeyMsg is kept on
// the .Key field so Models can fall back to raw inspection if needed.
func TestKeyActionMsg_FieldsPreserveKey(t *testing.T) {
	t.Parallel()
	raw := KeyMsg{Code: terminal.KeyRune, Rune: 'X', Mod: terminal.ModShift}
	wrapped := KeyActionMsg{Key: raw, Action: keybindings.ActionInsertRune}
	if wrapped.Key != raw {
		t.Errorf("Key field mutated: got %+v; want %+v", wrapped.Key, raw)
	}
	if wrapped.Action != keybindings.ActionInsertRune {
		t.Errorf("Action field = %v; want InsertRune", wrapped.Action)
	}
}
